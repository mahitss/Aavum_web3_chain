mod blockchain;
mod compute;
mod facilitator;
mod protocol;
mod subcontract;
mod voucher;

use axum::extract::{ConnectInfo, DefaultBodyLimit, Request, State};
use axum::http::header::CONTENT_TYPE;
use axum::http::{HeaderMap, HeaderValue, StatusCode};
use axum::middleware::{self, Next};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Json, Router};
use blockchain::BlockchainClient;
use protocol::{
    calculate_task_hash, decode_header, encode_header, hash_bytes, meter_usage, resolve_service,
    service_price, token_count, ComputeRequest, ComputeResponse, MeteredCall, PaymentRequired,
    PaymentRequiredError, PaymentResponseHeader, PaymentSignature, Provenance, X402Required,
    DEFAULT_RPC_URL, DEFAULT_TUSD8_ADDRESS, EXPLORER_TX_PREFIX, HOP_MARGIN, PAYMENT_REQUIRED,
    PAYMENT_RESPONSE, PAYMENT_SIGNATURE, PRICE_AI_SYNTHESIS, PRICE_CHAIN_AUDIT,
    PRICE_DEEP_DUE_DILIGENCE, PRICE_UPTO_CEILING, PRICE_WEB_BRIEF, PROTOCOL_VERSION,
    TASK_TIMEOUT_BLOCKS, X_ESCROW_TASK_ID, X_ESCROW_TASK_ID_LEGACY, X_PAYMENT_AMOUNT,
    X_PAYMENT_PROTOCOL, X_PAYMENT_RECEIVER, X_PAYMENT_TOKEN, X_TASK_HASH, X_TASK_TIMEOUT_BLOCKS,
};
use std::collections::{HashMap, HashSet};
use std::net::SocketAddr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant};
use tower_http::cors::{AllowOrigin, Any, CorsLayer};
use tracing::{error, info, warn, Instrument};

struct Counters {
    settled: AtomicU64,
    refunded: AtomicU64,
    failed: AtomicU64,
    blocked_cap: AtomicU64,
    blocked_revoked: AtomicU64,
    volume_units: AtomicU64,
    latency_ms: AtomicU64,
}

pub struct AppState {
    blockchain: BlockchainClient,
    inflight: RwLock<HashSet<[u8; 32]>>,
    hits: Mutex<HashMap<String, Vec<Instant>>>,
    counters: Counters,
    pub tabs: voucher::TabBook,
}

static REQUEST_IDS: AtomicU64 = AtomicU64::new(1);

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    dotenvy::dotenv().ok();

    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info,agent_worker=debug".into()),
        )
        .init();

    let rpc_url = std::env::var("AEVUM_RPC_URL").unwrap_or_else(|_| DEFAULT_RPC_URL.to_string());
    let private_key =
        std::env::var("WORKER_PRIVATE_KEY").expect("WORKER_PRIVATE_KEY must be provided");
    let escrow_address =
        std::env::var("NEXUS_ESCROW_ADDRESS").expect("NEXUS_ESCROW_ADDRESS must be provided");
    let port: u16 = std::env::var("WORKER_PORT")
        .or_else(|_| std::env::var("PORT"))
        .unwrap_or_else(|_| "8080".to_string())
        .parse()
        .expect("Valid WORKER_PORT required");

    info!("Initializing Nexus402 Rust Worker Node...");
    let blockchain = BlockchainClient::new(&rpc_url, &private_key, &escrow_address)?;
    info!("Worker Address: {}", blockchain.worker_address());
    info!("Target Escrow Contract: {}", blockchain.escrow_address());

    let state = Arc::new(AppState {
        blockchain,
        inflight: RwLock::new(HashSet::new()),
        hits: Mutex::new(HashMap::new()),
        counters: Counters {
            settled: AtomicU64::new(0),
            refunded: AtomicU64::new(0),
            failed: AtomicU64::new(0),
            blocked_cap: AtomicU64::new(0),
            blocked_revoked: AtomicU64::new(0),
            volume_units: AtomicU64::new(0),
            latency_ms: AtomicU64::new(0),
        },
        tabs: Mutex::new(HashMap::new()),
    });

    let cors = CorsLayer::new()
        .allow_origin(AllowOrigin::list(allowed_origins()))
        .allow_methods(Any)
        .allow_headers(Any)
        .expose_headers([
            X_PAYMENT_PROTOCOL.clone(),
            X_PAYMENT_TOKEN.clone(),
            X_PAYMENT_AMOUNT.clone(),
            X_PAYMENT_RECEIVER.clone(),
            X_TASK_HASH.clone(),
            X_TASK_TIMEOUT_BLOCKS.clone(),
            PAYMENT_REQUIRED.clone(),
            PAYMENT_RESPONSE.clone(),
            axum::http::HeaderName::from_static("x-request-id"),
        ]);

    let app = Router::new()
        .route("/health", get(health_handler))
        .route("/metrics", get(metrics_handler))
        .route("/.well-known/nexus402.json", get(catalog_handler))
        .route("/api/v1/compute", post(compute_handler))
        .route(
            "/api/v1/tabs/{tab_id}/settle",
            post(voucher::settle_handler),
        )
        .layer(cors)
        .layer(DefaultBodyLimit::max(64 * 1024))
        .layer(middleware::from_fn_with_state(state.clone(), rate_limit))
        .layer(middleware::from_fn(request_id))
        .with_state(state);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    info!("Worker server listening on http://{addr}");

    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(
        listener,
        app.into_make_service_with_connect_info::<SocketAddr>(),
    )
    .with_graceful_shutdown(shutdown_signal())
    .await?;
    Ok(())
}

fn allowed_origins() -> Vec<HeaderValue> {
    let raw =
        std::env::var("ALLOWED_ORIGINS").unwrap_or_else(|_| "http://localhost:3000".to_string());
    let mut origins = Vec::new();
    for part in raw.split(',') {
        let part = part.trim();
        if part.is_empty() || part == "*" {
            continue;
        }
        if let Ok(value) = HeaderValue::from_str(part) {
            origins.push(value);
        }
    }
    if origins.is_empty() {
        origins.push(HeaderValue::from_static("http://localhost:3000"));
    }
    origins
}

async fn request_id(request: Request, next: Next) -> Response {
    let incoming = request
        .headers()
        .get("x-request-id")
        .and_then(|value| value.to_str().ok())
        .filter(|value| !value.is_empty())
        .map(str::to_string);
    let id =
        incoming.unwrap_or_else(|| format!("{:x}", REQUEST_IDS.fetch_add(1, Ordering::Relaxed)));
    let span = tracing::info_span!("request", request_id = %id);
    let mut response = next.run(request).instrument(span).await;
    if let Ok(value) = HeaderValue::from_str(&id) {
        response.headers_mut().insert("x-request-id", value);
    }
    response
}

async fn rate_limit(State(state): State<Arc<AppState>>, request: Request, next: Next) -> Response {
    let ip = request
        .extensions()
        .get::<ConnectInfo<SocketAddr>>()
        .map(|info| info.0.ip().to_string())
        .unwrap_or_else(|| "unknown".to_string());
    if !allow_request(&state, &ip) {
        return (
            StatusCode::TOO_MANY_REQUESTS,
            Json(serde_json::json!({ "error": "rate limit exceeded" })),
        )
            .into_response();
    }
    next.run(request).await
}

fn allow_request(state: &AppState, ip: &str) -> bool {
    let mut hits = state.hits.lock().unwrap_or_else(|err| err.into_inner());
    let now = Instant::now();
    let window = hits.entry(ip.to_string()).or_default();
    window.retain(|seen| now.duration_since(*seen) < Duration::from_secs(10));
    if window.len() >= 30 {
        return false;
    }
    window.push(now);
    true
}

async fn shutdown_signal() {
    let ctrl_c = tokio::signal::ctrl_c();
    #[cfg(unix)]
    {
        let mut terminate =
            match tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()) {
                Ok(signal) => signal,
                Err(err) => {
                    warn!("SIGTERM handler unavailable: {err}");
                    ctrl_c.await.ok();
                    return;
                }
            };
        tokio::select! {
            _ = ctrl_c => {}
            _ = terminate.recv() => {}
        }
    }
    #[cfg(not(unix))]
    {
        ctrl_c.await.ok();
    }
    info!("Shutdown signal received. Finishing in-flight claims.");
}

struct FlightGuard {
    state: Arc<AppState>,
    id: [u8; 32],
}

impl Drop for FlightGuard {
    fn drop(&mut self) {
        let mut guard = self
            .state
            .inflight
            .write()
            .unwrap_or_else(|err| err.into_inner());
        guard.remove(&self.id);
    }
}

async fn catalog_handler(State(state): State<Arc<AppState>>) -> impl IntoResponse {
    Json(serde_json::json!({
        "worker": state.blockchain.worker_address().to_string(),
        "network": "eip155:44005",
        "protocol": "x402-v2",
        "schemes": ["exact", "upto", "batch-settlement", "auth-capture"],
        "services": [
            {"name": "web_brief", "price": "0.15", "priceUnits": PRICE_WEB_BRIEF.to_string(), "timeoutBlocks": TASK_TIMEOUT_BLOCKS, "input": "public https URL"},
            {"name": "ai_synthesis", "price": "0.25", "priceUnits": PRICE_AI_SYNTHESIS.to_string(), "schemes": ["exact", "upto"], "uptoUnits": PRICE_UPTO_CEILING.to_string(), "timeoutBlocks": TASK_TIMEOUT_BLOCKS, "input": "prompt text"},
            {"name": "chain_audit", "price": "0.50", "priceUnits": PRICE_CHAIN_AUDIT.to_string(), "timeoutBlocks": TASK_TIMEOUT_BLOCKS, "input": "0x address"},
            {"name": "deep_due_diligence", "price": "0.60", "priceUnits": PRICE_DEEP_DUE_DILIGENCE.to_string(), "timeoutBlocks": TASK_TIMEOUT_BLOCKS, "input": "address and optional https URL"},
            {"name": "micro_call", "price": "0.05", "priceUnits": crate::protocol::PRICE_MICRO_CALL.to_string(), "schemes": ["batch-settlement"], "timeoutBlocks": TASK_TIMEOUT_BLOCKS, "input": "short text"}
        ]
    }))
}

async fn metrics_handler(State(state): State<Arc<AppState>>) -> impl IntoResponse {
    let gas = match state.blockchain.native_balance().await {
        Ok(wei) => format_tqday(wei),
        Err(_) => "0.000000".to_string(),
    };
    let body = format!(
        "# HELP nexus402_tasks_total Tasks observed by this process.\n# TYPE nexus402_tasks_total counter\nnexus402_tasks_total{{status=\"settled\"}} {}\nnexus402_tasks_total{{status=\"refunded\"}} {}\nnexus402_tasks_total{{status=\"failed\"}} {}\nnexus402_tasks_total{{status=\"blocked_cap\"}} {}\nnexus402_tasks_total{{status=\"blocked_revoked\"}} {}\n# HELP nexus402_tusd8_volume_units Settled tUSD8 base units.\n# TYPE nexus402_tusd8_volume_units counter\nnexus402_tusd8_volume_units {}\n# HELP nexus402_settlement_latency_ms Most recent settlement latency.\n# TYPE nexus402_settlement_latency_ms gauge\nnexus402_settlement_latency_ms {}\n# HELP nexus402_wallet_gas_balance_tqday Worker tQDAY balance.\n# TYPE nexus402_wallet_gas_balance_tqday gauge\nnexus402_wallet_gas_balance_tqday {}\n",
        state.counters.settled.load(Ordering::Relaxed),
        state.counters.refunded.load(Ordering::Relaxed),
        state.counters.failed.load(Ordering::Relaxed),
        state.counters.blocked_cap.load(Ordering::Relaxed),
        state.counters.blocked_revoked.load(Ordering::Relaxed),
        state.counters.volume_units.load(Ordering::Relaxed),
        state.counters.latency_ms.load(Ordering::Relaxed),
        gas
    );
    (
        [(CONTENT_TYPE, "text/plain; version=0.0.4; charset=utf-8")],
        body,
    )
}

fn format_tqday(wei: alloy::primitives::U256) -> String {
    let micro = (wei / alloy::primitives::U256::from(1_000_000_000_000u64)).to::<u128>();
    format!("{}.{:06}", micro / 1_000_000, micro % 1_000_000)
}

async fn health_handler(State(state): State<Arc<AppState>>) -> impl IntoResponse {
    Json(serde_json::json!({
        "status": "healthy",
        "worker": state.blockchain.worker_address().to_string(),
        "network": "QDay Aevum Testnet (Chain ID 44005)"
    }))
}

async fn compute_handler(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Json(payload): Json<ComputeRequest>,
) -> Response {
    let service = match resolve_service(&payload) {
        Ok(service) => service,
        Err(err) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(serde_json::json!({ "error": err })),
            )
                .into_response();
        }
    };
    let faults = std::env::var("ENABLE_DEMO_FAULTS")
        .map(|value| matches!(value.as_str(), "1" | "true" | "TRUE" | "yes"))
        .unwrap_or(false);
    let header_fault = headers
        .get("x-simulate-timeout")
        .and_then(|value| value.to_str().ok())
        .map(|value| matches!(value, "1" | "true" | "TRUE"))
        .unwrap_or(false);
    let simulate_timeout = faults && (payload.simulate_timeout || header_fault);
    let header_garbage = headers
        .get("x-simulate-garbage")
        .and_then(|value| value.to_str().ok())
        .map(|value| matches!(value, "1" | "true" | "TRUE"))
        .unwrap_or(false);
    let simulate_garbage = faults && (payload.simulate_garbage || header_garbage);
    let timeout_blocks = if simulate_timeout {
        5
    } else {
        TASK_TIMEOUT_BLOCKS
    };
    let price = service_price(service);
    let task_hash = calculate_task_hash(&payload);
    let task_hash_hex = format!("0x{}", hex::encode(task_hash));

    let signed = headers
        .get(PAYMENT_SIGNATURE.clone())
        .and_then(|value| value.to_str().ok())
        .and_then(|value| decode_header::<PaymentSignature>(value).ok());
    let escrow_task_id = signed
        .as_ref()
        .map(|payment| payment.task_id.clone())
        .or_else(|| {
            headers
                .get(X_ESCROW_TASK_ID.clone())
                .or_else(|| headers.get(X_ESCROW_TASK_ID_LEGACY.clone()))
                .and_then(|value| value.to_str().ok())
                .map(str::to_string)
        })
        .map(|value| value.trim().trim_start_matches("0x").to_string())
        .filter(|value| !value.is_empty());

    if signed
        .as_ref()
        .is_some_and(|payment| payment.scheme == "batch-settlement")
    {
        return voucher::accept(
            state,
            &headers,
            &payload,
            service,
            signed.as_ref().expect("batch signature"),
        )
        .await;
    }

    let Some(task_id_hex) = escrow_task_id else {
        warn!(
            "Unpaid request received. Emitting HTTP 402 challenge for task_hash: {task_hash_hex}"
        );
        let error_body = PaymentRequiredError {
            error: "Payment Required".to_string(),
            message: "Compute requires conditional tUSD8 micro-escrow lock on Aevum Testnet"
                .to_string(),
            required_amount: price,
            token: DEFAULT_TUSD8_ADDRESS.to_string(),
            task_hash: task_hash_hex.clone(),
        };
        let mut response = (StatusCode::PAYMENT_REQUIRED, Json(error_body)).into_response();
        let response_headers = response.headers_mut();
        response_headers.insert(
            X_PAYMENT_PROTOCOL.clone(),
            PROTOCOL_VERSION.parse().unwrap(),
        );
        response_headers.insert(
            X_PAYMENT_TOKEN.clone(),
            DEFAULT_TUSD8_ADDRESS.parse().unwrap(),
        );
        response_headers.insert(X_PAYMENT_AMOUNT.clone(), price.to_string().parse().unwrap());
        let pay_to = state.blockchain.worker_address().to_string();
        let mut accepts = vec![
            PaymentRequired {
                scheme: "exact".to_string(),
                network: "eip155:44005".to_string(),
                asset: DEFAULT_TUSD8_ADDRESS.to_string(),
                amount: price.to_string(),
                pay_to: pay_to.clone(),
                task_hash: task_hash_hex.clone(),
                timeout_blocks,
            },
            PaymentRequired {
                scheme: "nexus-escrow-v1".to_string(),
                network: "eip155:44005".to_string(),
                asset: DEFAULT_TUSD8_ADDRESS.to_string(),
                amount: price.to_string(),
                pay_to: pay_to.clone(),
                task_hash: task_hash_hex.clone(),
                timeout_blocks,
            },
        ];
        accepts.push(PaymentRequired {
            scheme: "auth-capture".to_string(),
            network: "eip155:44005".to_string(),
            asset: DEFAULT_TUSD8_ADDRESS.to_string(),
            amount: price.to_string(),
            pay_to: pay_to.clone(),
            task_hash: task_hash_hex.clone(),
            timeout_blocks,
        });
        accepts.push(PaymentRequired {
            scheme: "batch-settlement".to_string(),
            network: "eip155:44005".to_string(),
            asset: DEFAULT_TUSD8_ADDRESS.to_string(),
            amount: price.to_string(),
            pay_to: pay_to.clone(),
            task_hash: task_hash_hex.clone(),
            timeout_blocks,
        });
        if service == "ai_synthesis" {
            accepts.push(PaymentRequired {
                scheme: "upto".to_string(),
                network: "eip155:44005".to_string(),
                asset: DEFAULT_TUSD8_ADDRESS.to_string(),
                amount: PRICE_UPTO_CEILING.to_string(),
                pay_to,
                task_hash: task_hash_hex.clone(),
                timeout_blocks,
            });
        }
        let wire = X402Required {
            x402_version: 2,
            accepts,
        };
        if let Ok(value) = encode_header(&wire).parse() {
            response_headers.insert(PAYMENT_REQUIRED.clone(), value);
        }
        response_headers.insert(
            X_PAYMENT_RECEIVER.clone(),
            state
                .blockchain
                .worker_address()
                .to_string()
                .parse()
                .unwrap(),
        );
        response_headers.insert(X_TASK_HASH.clone(), task_hash_hex.parse().unwrap());
        response_headers.insert(
            X_TASK_TIMEOUT_BLOCKS.clone(),
            timeout_blocks.to_string().parse().unwrap(),
        );
        return response;
    };

    let task_id_bytes = match BlockchainClient::parse_task_id(&task_id_hex) {
        Ok(bytes) => bytes,
        Err(err) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(serde_json::json!({ "error": err.to_string() })),
            )
                .into_response();
        }
    };

    {
        let mut inflight = state
            .inflight
            .write()
            .unwrap_or_else(|err| err.into_inner());
        if !inflight.insert(task_id_bytes) {
            return (
                StatusCode::CONFLICT,
                Json(serde_json::json!({
                    "error": "task claim already in flight",
                    "task_id": format!("0x{task_id_hex}")
                })),
            )
                .into_response();
        }
    }
    let _flight = FlightGuard {
        state: state.clone(),
        id: task_id_bytes,
    };

    let metered = payload.scheme == "upto" && service == "ai_synthesis";
    let required_units = if metered { PRICE_UPTO_CEILING } else { price };
    let facilitator_url = std::env::var("FACILITATOR_URL")
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty());
    info!("Validating on-chain escrow lock for taskId: 0x{task_id_hex}");
    let verification = if let Some(url) = facilitator_url.as_deref() {
        facilitator::verify_with_facilitator(
            url,
            &task_id_hex,
            &task_hash_hex,
            required_units,
            &state.blockchain.worker_address().to_string(),
        )
        .await
    } else {
        state
            .blockchain
            .verify_escrow(task_id_bytes, task_hash, required_units)
            .await
    };
    if let Err(err) = verification {
        error!("Escrow verification failed: {err}");
        return (
            StatusCode::PAYMENT_REQUIRED,
            Json(serde_json::json!({
                "error": "Escrow verification failed",
                "details": err.to_string()
            })),
        )
            .into_response();
    }

    let started = Instant::now();
    if simulate_timeout {
        state.counters.failed.fetch_add(1, Ordering::Relaxed);
        info!("Demo fault armed. Leaving task 0x{task_id_hex} pending for the refund watchdog.");
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
        return (
            StatusCode::GATEWAY_TIMEOUT,
            Json(serde_json::json!({
                "error": "simulated worker timeout",
                "task_id": format!("0x{task_id_hex}")
            })),
        )
            .into_response();
    }

    info!("Escrow confirmed on Aevum Testnet. Running compute...");
    let mut provenance = None;
    let prompts = if service == "deep_due_diligence" {
        Vec::new()
    } else if payload.batch.is_empty() {
        vec![payload.prompt.clone()]
    } else {
        payload.batch.clone()
    };
    let mut outputs = Vec::with_capacity(prompts.len().max(1));
    let mut breakdown = Vec::new();
    if simulate_garbage {
        outputs.push(String::new());
    } else if service == "deep_due_diligence" {
        let sub_url = std::env::var("SUB_WORKER_URL").unwrap_or_default();
        let principal_hex = std::env::var("SUB_PRINCIPAL_ADDRESS").unwrap_or_default();
        if sub_url.trim().is_empty() || principal_hex.trim().is_empty() {
            state.counters.failed.fetch_add(1, Ordering::Relaxed);
            return (
                StatusCode::BAD_GATEWAY,
                Json(serde_json::json!({
                    "error": "multi-hop subcontracting is not configured",
                    "details": "Set SUB_WORKER_URL and SUB_PRINCIPAL_ADDRESS. The worker address must already hold a scoped session for that principal."
                })),
            )
                .into_response();
        }
        let principal = match principal_hex.trim().parse() {
            Ok(address) => address,
            Err(err) => {
                return (
                    StatusCode::BAD_GATEWAY,
                    Json(serde_json::json!({"error": format!("SUB_PRINCIPAL_ADDRESS: {err}")})),
                )
                    .into_response();
            }
        };
        match subcontract::hire_children(
            &state.blockchain,
            principal,
            sub_url.trim(),
            &payload.prompt,
        )
        .await
        {
            Ok((chain_output, web_output, sub_tasks)) => {
                provenance = Some(Provenance {
                    parent_task_id: format!("0x{task_id_hex}"),
                    parent_amount: PRICE_DEEP_DUE_DILIGENCE.to_string(),
                    margin: HOP_MARGIN.to_string(),
                    sub_tasks,
                });
                outputs.push(subcontract::synthesize(&chain_output, &web_output));
            }
            Err(err) => {
                state.counters.failed.fetch_add(1, Ordering::Relaxed);
                return (
                    StatusCode::BAD_GATEWAY,
                    Json(serde_json::json!({
                        "error": "sub-contract failed before the parent claim",
                        "details": err
                    })),
                )
                    .into_response();
            }
        }
    }
    if !simulate_garbage {
        for prompt in &prompts {
            let mut item = payload.clone();
            item.prompt = prompt.clone();
            item.batch.clear();
            match compute::execute_service(&item, &state.blockchain).await {
                Ok((output, _)) => {
                    if metered {
                        let amount = meter_usage(&output);
                        breakdown.push(MeteredCall {
                            prompt: prompt.clone(),
                            tokens: token_count(&output),
                            amount: amount.to_string(),
                        });
                    }
                    outputs.push(output);
                }
                Err(err) => {
                    state.counters.failed.fetch_add(1, Ordering::Relaxed);
                    return (
                        StatusCode::BAD_GATEWAY,
                        Json(serde_json::json!({
                            "error": "compute service failed before claim",
                            "details": err
                        })),
                    )
                        .into_response();
                }
            }
        }
    }
    let output = if simulate_garbage {
        "GARBAGE::{not-json".to_string()
    } else if metered {
        serde_json::to_string(&outputs).unwrap_or_default()
    } else {
        outputs.first().cloned().unwrap_or_default()
    };
    let result_hash = hash_bytes(output.as_bytes());
    let result_hash_hex = format!("0x{}", hex::encode(result_hash));
    let evaluated = payload.scheme == "auth-capture"
        || signed
            .as_ref()
            .is_some_and(|payment| payment.scheme == "auth-capture");
    if evaluated {
        info!("Submitting work for evaluator review on taskId: 0x{task_id_hex}");
        match state
            .blockchain
            .submit_for_evaluation(task_id_bytes, result_hash)
            .await
        {
            Ok(submit_tx) => {
                state
                    .counters
                    .latency_ms
                    .store(started.elapsed().as_millis() as u64, Ordering::Relaxed);
                let response = ComputeResponse {
                    status: "SUBMITTED_FOR_EVALUATION".to_string(),
                    result: output,
                    task_id: format!("0x{task_id_hex}"),
                    task_hash: task_hash_hex,
                    result_hash: result_hash_hex,
                    claim_tx_hash: submit_tx,
                    ceiling: None,
                    actual_amount: None,
                    refunded_change: None,
                    breakdown: None,
                    provenance: None,
                };
                return (StatusCode::OK, Json(response)).into_response();
            }
            Err(err) => {
                state.counters.failed.fetch_add(1, Ordering::Relaxed);
                error!("Failed to submit work for evaluation: {err}");
                return (
                    StatusCode::BAD_GATEWAY,
                    Json(serde_json::json!({
                        "error": "submitWorkForEvaluation failed",
                        "details": err.to_string()
                    })),
                )
                    .into_response();
            }
        }
    }

    info!("Submitting cryptographic execution proof to NexusEscrow...");
    let (claim_tx, settled_units, ceiling, actual_amount, refunded_change) = if metered {
        let locked = match state.blockchain.escrow_amount(task_id_bytes).await {
            Ok(amount) => amount,
            Err(err) => {
                state.counters.failed.fetch_add(1, Ordering::Relaxed);
                return (
                    StatusCode::BAD_GATEWAY,
                    Json(serde_json::json!({"error": err.to_string()})),
                )
                    .into_response();
            }
        };
        let summed: u64 = breakdown
            .iter()
            .filter_map(|call| call.amount.parse::<u64>().ok())
            .sum();
        let actual = if locked == 0 {
            1
        } else {
            summed.clamp(1, locked)
        };
        let refund = locked.saturating_sub(actual);
        let claim = if let Some(url) = facilitator_url.as_deref() {
            facilitator::settle_with_facilitator(
                &state.blockchain,
                url,
                task_id_bytes,
                result_hash,
                Some(actual),
            )
            .await
        } else {
            state
                .blockchain
                .claim_metered(task_id_bytes, actual, result_hash)
                .await
        };
        match claim {
            Ok(tx_hash) => (
                tx_hash,
                actual,
                Some(locked.to_string()),
                Some(actual.to_string()),
                Some(refund.to_string()),
            ),
            Err(err) => {
                state.counters.failed.fetch_add(1, Ordering::Relaxed);
                error!("Failed to claim metered payout: {err}");
                return (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json(serde_json::json!({
                        "error": "Failed to claim escrow payout on-chain",
                        "details": err.to_string()
                    })),
                )
                    .into_response();
            }
        }
    } else {
        let claim = if let Some(url) = facilitator_url.as_deref() {
            facilitator::settle_with_facilitator(
                &state.blockchain,
                url,
                task_id_bytes,
                result_hash,
                None,
            )
            .await
        } else {
            state
                .blockchain
                .claim_payout(task_id_bytes, result_hash)
                .await
        };
        match claim {
            Ok(tx_hash) => (tx_hash, price, None, None, None),
            Err(err) => {
                state.counters.failed.fetch_add(1, Ordering::Relaxed);
                error!("Failed to claim on-chain payout: {err}");
                return (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json(serde_json::json!({
                        "error": "Failed to claim escrow payout on-chain",
                        "details": err.to_string()
                    })),
                )
                    .into_response();
            }
        }
    };

    info!("Payout claimed! Tx Hash: {claim_tx}");
    state.counters.settled.fetch_add(1, Ordering::Relaxed);
    state
        .counters
        .volume_units
        .fetch_add(settled_units, Ordering::Relaxed);
    state
        .counters
        .latency_ms
        .store(started.elapsed().as_millis() as u64, Ordering::Relaxed);
    let response = ComputeResponse {
        status: "COMPLETED".to_string(),
        result: output,
        task_id: format!("0x{task_id_hex}"),
        task_hash: task_hash_hex,
        result_hash: result_hash_hex.clone(),
        claim_tx_hash: claim_tx.clone(),
        ceiling,
        actual_amount: actual_amount.clone(),
        refunded_change: refunded_change.clone(),
        breakdown: if breakdown.is_empty() {
            None
        } else {
            Some(breakdown)
        },
        provenance,
    };
    let mut response = (StatusCode::OK, Json(response)).into_response();
    let settled = PaymentResponseHeader {
        x402_version: 2,
        status: "settled".to_string(),
        claim_tx_hash: claim_tx.clone(),
        result_hash: result_hash_hex.clone(),
        explorer_url: format!("{EXPLORER_TX_PREFIX}{claim_tx}"),
        actual_amount,
        refunded_change,
    };
    if let Ok(value) = encode_header(&settled).parse() {
        response
            .headers_mut()
            .insert(PAYMENT_RESPONSE.clone(), value);
    }
    response
}
