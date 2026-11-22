mod chain;
mod compute;
mod paywall;

use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;

use alloy::network::EthereumWallet;
use alloy::primitives::{keccak256, Address, U256};
use alloy::providers::ProviderBuilder;
use alloy::signers::local::PrivateKeySigner;
use alloy::signers::Signer;
use anyhow::Context;
use axum::body::Body;
use axum::extract::{Request, State};
use axum::http::{HeaderMap, StatusCode};
use axum::middleware::{from_fn, Next};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Extension, Json, Router};
use chain::verify_and_claim;
use paywall::{payment_required, PaymentChallenge, PROTOCOL};
use serde::Serialize;
use serde_json::{json, Value};
use tower_http::cors::{Any, CorsLayer};
use tracing::info;

#[derive(Clone)]
struct Settings {
    escrow: Address,
    signer: PrivateKeySigner,
    amount: U256,
    timeout_blocks: u64,
    bind: SocketAddr,
    rpc_url: String,
    chain_id: u64,
}

#[derive(Clone)]
struct AppState<P> {
    settings: Arc<Settings>,
    provider: P,
}

#[derive(Clone, Copy)]
struct PayloadHash(alloy::primitives::B256);

#[derive(Serialize)]
struct ComputeResponse {
    result: Value,
    #[serde(rename = "resultHash")]
    result_hash: String,
    #[serde(rename = "claimTx")]
    claim_tx: String,
    #[serde(rename = "taskId")]
    task_id: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let _ = rustls::crypto::ring::default_provider().install_default();
    dotenvy::dotenv().ok();
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    let settings = Arc::new(load_settings()?);
    let mut signer = settings.signer.clone();
    signer.set_chain_id(Some(settings.chain_id));
    let wallet = EthereumWallet::from(signer);
    let rpc = settings.rpc_url.parse().context("RPC_URL")?;
    let provider = ProviderBuilder::new().wallet(wallet).connect_http(rpc);
    let state = AppState {
        settings: settings.clone(),
        provider,
    };

    let app = Router::new()
        .route("/health", get(health))
        .route("/api/v1/compute", post(compute))
        .layer(from_fn(paywall))
        .layer(Extension(settings.clone()))
        .layer(
            CorsLayer::new()
                .allow_origin(Any)
                .allow_methods(Any)
                .allow_headers(Any)
                .expose_headers(Any),
        )
        .with_state(state);

    info!(
        receiver = %settings.signer.address(),
        escrow = %settings.escrow,
        bind = %settings.bind,
        "nexus402 worker listening"
    );
    let listener = tokio::net::TcpListener::bind(settings.bind).await?;
    axum::serve(listener, app).await?;
    Ok(())
}

fn load_settings() -> anyhow::Result<Settings> {
    let signer: PrivateKeySigner = std::env::var("WORKER_PRIVATE_KEY")
        .context("WORKER_PRIVATE_KEY")?
        .trim()
        .parse()
        .context("parse WORKER_PRIVATE_KEY")?;
    let escrow = std::env::var("ESCROW_ADDRESS")
        .context("ESCROW_ADDRESS")?
        .trim()
        .parse()
        .context("parse ESCROW_ADDRESS")?;
    let amount: u64 = std::env::var("PAYMENT_AMOUNT")
        .unwrap_or_else(|_| "250000".to_string())
        .parse()
        .context("PAYMENT_AMOUNT")?;
    let timeout_blocks = std::env::var("TASK_TIMEOUT_BLOCKS")
        .unwrap_or_else(|_| "50".to_string())
        .parse()
        .context("TASK_TIMEOUT_BLOCKS")?;
    let bind: SocketAddr = std::env::var("BIND_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:4020".to_string())
        .parse()
        .context("BIND_ADDR")?;
    Ok(Settings {
        escrow,
        signer,
        amount: U256::from(amount),
        timeout_blocks,
        bind,
        rpc_url: std::env::var("RPC_URL")
            .unwrap_or_else(|_| "https://rpc-test.qday.info".to_string()),
        chain_id: std::env::var("CHAIN_ID")
            .unwrap_or_else(|_| "44005".to_string())
            .parse()
            .context("CHAIN_ID")?,
    })
}

async fn health<P>(State(state): State<AppState<P>>) -> Json<Value>
where
    P: Send + Sync + 'static,
{
    Json(json!({
        "status": "ok",
        "protocol": PROTOCOL,
        "receiver": format!("{}", state.settings.signer.address()),
        "escrow": format!("{}", state.settings.escrow),
        "amount": state.settings.amount.to_string(),
        "chainId": state.settings.chain_id,
    }))
}

async fn paywall(request: Request, next: Next) -> Response {
    if request.uri().path() != "/api/v1/compute" {
        return next.run(request).await;
    }
    let settings = match request.extensions().get::<Arc<Settings>>().cloned() {
        Some(settings) => settings,
        None => {
            return (StatusCode::INTERNAL_SERVER_ERROR, "paywall is not configured").into_response()
        }
    };
    let (parts, body) = request.into_parts();
    let bytes = match axum::body::to_bytes(body, 1024 * 1024).await {
        Ok(bytes) => bytes,
        Err(_) => return (StatusCode::BAD_REQUEST, "unreadable body").into_response(),
    };
    let task_hash = keccak256(bytes.as_ref());
    if !parts.headers.contains_key("x-escrow-taskid") {
        return payment_required(&PaymentChallenge {
            receiver: settings.signer.address(),
            amount: settings.amount,
            task_hash,
            timeout_blocks: settings.timeout_blocks,
        });
    }
    let mut request = Request::from_parts(parts, Body::from(bytes));
    request.extensions_mut().insert(PayloadHash(task_hash));
    next.run(request).await
}

async fn compute<P>(
    State(state): State<AppState<P>>,
    Extension(PayloadHash(task_hash)): Extension<PayloadHash>,
    headers: HeaderMap,
    body: axum::body::Bytes,
) -> Response
where
    P: alloy::providers::Provider + Clone + Send + Sync + 'static,
{
    let Some(task_header) = headers.get("x-escrow-taskid").and_then(|v| v.to_str().ok()) else {
        return (StatusCode::PAYMENT_REQUIRED, "missing X-Escrow-TaskId").into_response();
    };
    let task_id = match task_header.trim().parse::<alloy::primitives::B256>() {
        Ok(id) => id,
        Err(_) => return (StatusCode::BAD_REQUEST, "invalid X-Escrow-TaskId").into_response(),
    };

    let claim = verify_and_claim(
        &state.provider,
        &state.settings.signer,
        state.settings.escrow,
        state.settings.signer.address(),
        state.settings.amount,
        task_id,
        body.as_ref(),
        task_hash,
    );
    match tokio::time::timeout(Duration::from_secs(90), claim).await {
        Ok(Ok(claim)) => Json(ComputeResponse {
            result: claim.result,
            result_hash: format!("{}", claim.result_hash),
            claim_tx: format!("{}", claim.claim_tx),
            task_id: format!("{task_id}"),
        })
        .into_response(),
        Ok(Err(err)) => (StatusCode::CONFLICT, err.to_string()).into_response(),
        Err(_) => (StatusCode::GATEWAY_TIMEOUT, "claim timed out").into_response(),
    }
}

