mod blockchain;
mod compute;
mod protocol;

use axum::extract::State;
use axum::http::{HeaderMap, StatusCode};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Json, Router};
use blockchain::BlockchainClient;
use protocol::*;
use std::net::SocketAddr;
use std::sync::Arc;
use tower_http::cors::{Any, CorsLayer};
use tracing::{error, info, warn};

#[derive(Clone)]
struct AppState {
    blockchain: BlockchainClient,
}

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
    let private_key = std::env::var("WORKER_PRIVATE_KEY").expect("WORKER_PRIVATE_KEY must be provided");
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

    let state = Arc::new(AppState { blockchain });

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
        ]);

    let app = Router::new()
        .route("/health", get(health_handler))
        .route("/api/v1/compute", post(compute_handler))
        .layer(cors)
        .with_state(state);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    info!("Worker server listening on http://{addr}");

    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;
    Ok(())
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
    let task_hash = calculate_task_hash(&payload);
    let task_hash_hex = format!("0x{}", hex::encode(task_hash));

    let escrow_task_id = headers
        .get(X_ESCROW_TASK_ID.clone())
        .or_else(|| headers.get(X_ESCROW_TASK_ID_LEGACY.clone()))
        .and_then(|value| value.to_str().ok())
        .map(|value| value.trim().trim_start_matches("0x"))
        .filter(|value| !value.is_empty());

    let Some(task_id_hex) = escrow_task_id else {
        warn!(
            "Unpaid request received. Emitting HTTP 402 challenge for task_hash: {task_hash_hex}"
        );
        let error_body = PaymentRequiredError {
            error: "Payment Required".to_string(),
            message: "Compute requires conditional tUSD8 micro-escrow lock on Aevum Testnet"
                .to_string(),
            required_amount: TASK_PRICE_UNITS,
            token: DEFAULT_TUSD8_ADDRESS.to_string(),
            task_hash: task_hash_hex.clone(),
        };
        let mut response = (StatusCode::PAYMENT_REQUIRED, Json(error_body)).into_response();
        let response_headers = response.headers_mut();
        response_headers.insert(X_PAYMENT_PROTOCOL.clone(), PROTOCOL_VERSION.parse().unwrap());
        response_headers.insert(
            X_PAYMENT_TOKEN.clone(),
            DEFAULT_TUSD8_ADDRESS.parse().unwrap(),
        );
        response_headers.insert(
            X_PAYMENT_AMOUNT.clone(),
            TASK_PRICE_UNITS.to_string().parse().unwrap(),
        );
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

    let task_id_bytes = match BlockchainClient::parse_task_id(task_id_hex) {
        Ok(bytes) => bytes,
        Err(err) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(serde_json::json!({ "error": err.to_string() })),
            )
                .into_response();
        }
    };

    info!("Validating on-chain escrow lock for taskId: 0x{task_id_hex}");
    if let Err(err) = state.blockchain.verify_escrow(task_id_bytes, task_hash).await {
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
    let (output, result_hash) = compute::execute_compute_job(&payload);
    let result_hash_hex = format!("0x{}", hex::encode(result_hash));

    info!("Submitting cryptographic execution proof to NexusEscrow...");
    let claim_tx = match state.blockchain.claim_payout(task_id_bytes, result_hash).await {
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
        result_hash: result_hash_hex,
        claim_tx_hash: claim_tx,
    };
    (StatusCode::OK, Json(response)).into_response()
}
