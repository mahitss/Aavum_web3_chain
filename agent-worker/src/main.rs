mod blockchain;
mod compute;
mod protocol;

use axum::extract::{ConnectInfo, DefaultBodyLimit, Request, State};
use axum::http::{HeaderMap, HeaderValue, StatusCode};
use axum::middleware::{self, Next};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Json, Router};
use blockchain::BlockchainClient;
use protocol::{
    calculate_task_hash, decode_header, encode_header, resolve_service, service_price,
    ComputeRequest, ComputeResponse, PaymentRequired, PaymentRequiredError, PaymentResponseHeader,
    PaymentSignature, DEFAULT_RPC_URL, DEFAULT_TUSD8_ADDRESS, EXPLORER_TX_PREFIX, PAYMENT_REQUIRED,
    PAYMENT_RESPONSE, PAYMENT_SIGNATURE, PROTOCOL_VERSION, TASK_TIMEOUT_BLOCKS, X_ESCROW_TASK_ID,
    X_ESCROW_TASK_ID_LEGACY, X_PAYMENT_AMOUNT, X_PAYMENT_PROTOCOL, X_PAYMENT_RECEIVER,
    X_PAYMENT_TOKEN, X_TASK_HASH, X_TASK_TIMEOUT_BLOCKS,
};
use std::collections::{HashMap, HashSet};
use std::net::SocketAddr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant};
use tower_http::cors::{Any, CorsLayer};
use tracing::{error, info, warn, Instrument};

struct AppState {
    blockchain: BlockchainClient,
    inflight: RwLock<HashSet<[u8; 32]>>,
    hits: Mutex<HashMap<String, Vec<Instant>>>,
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
    });

    let cors = CorsLayer::new()
        .allow_origin(Any)
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
        .route("/api/v1/compute", post(compute_handler))
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
        let challenge = PaymentRequired {
            scheme: "nexus-escrow-v1".to_string(),
            network: "eip155:44005".to_string(),
            asset: DEFAULT_TUSD8_ADDRESS.to_string(),
            amount: price.to_string(),
            pay_to: state.blockchain.worker_address().to_string(),
            task_hash: task_hash_hex.clone(),
            timeout_blocks: TASK_TIMEOUT_BLOCKS,
        };
        if let Ok(value) = encode_header(&challenge).parse() {
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
            TASK_TIMEOUT_BLOCKS.to_string().parse().unwrap(),
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

    info!("Validating on-chain escrow lock for taskId: 0x{task_id_hex}");
    if let Err(err) = state
        .blockchain
        .verify_escrow(task_id_bytes, task_hash, price)
        .await
    {
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

    info!("Escrow confirmed on Aevum Testnet. Running compute...");
    let (output, result_hash) = match compute::execute_service(&payload, &state.blockchain).await {
        Ok(result) => result,
        Err(err) => {
            return (
                StatusCode::BAD_GATEWAY,
                Json(serde_json::json!({
                    "error": "compute service failed before claim",
                    "details": err
                })),
            )
                .into_response();
        }
    };
    let result_hash_hex = format!("0x{}", hex::encode(result_hash));

    info!("Submitting cryptographic execution proof to NexusEscrow...");
    let claim_tx = match state
        .blockchain
        .claim_payout(task_id_bytes, result_hash)
        .await
    {
        Ok(tx_hash) => tx_hash,
        Err(err) => {
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
    };

    info!("Payout claimed! Tx Hash: {claim_tx}");
    let response = ComputeResponse {
        status: "COMPLETED".to_string(),
        result: output,
        task_id: format!("0x{task_id_hex}"),
        task_hash: task_hash_hex,
        result_hash: result_hash_hex.clone(),
        claim_tx_hash: claim_tx.clone(),
    };
    let mut response = (StatusCode::OK, Json(response)).into_response();
    let settled = PaymentResponseHeader {
        status: "settled".to_string(),
        claim_tx_hash: claim_tx.clone(),
        result_hash: result_hash_hex.clone(),
        explorer_url: format!("{EXPLORER_TX_PREFIX}{claim_tx}"),
    };
    if let Ok(value) = encode_header(&settled).parse() {
        response
            .headers_mut()
            .insert(PAYMENT_RESPONSE.clone(), value);
    }
    response
}
