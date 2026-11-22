mod blockchain;
mod compute;
mod middleware;
mod protocol;

use std::net::SocketAddr;

use axum::extract::{Extension, State};
use axum::http::StatusCode;
use axum::middleware::from_fn_with_state;
use axum::response::IntoResponse;
use axum::routing::{get, post};
use axum::{Json, Router};
use serde_json::json;
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;
use tracing::info;

use blockchain::EscrowEngine;
use compute::execute_task;
use middleware::{require_escrow, PaywallState};
use protocol::{hex_hash, keccak256, ComputeRequest, ComputeResponse, EscrowTaskId};

#[derive(Clone)]
struct AppState {
    engine: EscrowEngine,
    worker: String,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    dotenv::dotenv().ok();
    tracing_subscriber::fmt::init();

    let private_key = std::env::var("WORKER_PRIVATE_KEY")?;
    let escrow = std::env::var("NEXUS_ESCROW_ADDRESS")?;
    let port: u16 = std::env::var("PORT")
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(8080);

    let engine = EscrowEngine::connect(&private_key, &escrow)?;
    let worker = format!("{:#x}", engine.worker_address());
    info!(%worker, port, "nexus402 worker listening");

    let paywall = PaywallState {
        receiver: worker.clone(),
    };
    let app = Router::new()
        .route("/health", get(health))
        .route(
            "/api/v1/compute",
            post(compute).layer(from_fn_with_state(paywall, require_escrow)),
        )
        .layer(CorsLayer::permissive())
        .layer(TraceLayer::new_for_http())
        .with_state(AppState { engine, worker });

    let address = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = tokio::net::TcpListener::bind(address).await?;
    axum::serve(listener, app).await?;
    Ok(())
}

async fn health(State(state): State<AppState>) -> Json<serde_json::Value> {
    Json(json!({
        "status": "ok",
        "worker": state.worker,
    }))
}

async fn compute(
    State(state): State<AppState>,
    Extension(EscrowTaskId(task_id)): Extension<EscrowTaskId>,
    body: axum::body::Bytes,
) -> axum::response::Response {
    let expected_task_hash = keccak256(&body);
    let request: ComputeRequest = match serde_json::from_slice(&body) {
        Ok(request) => request,
        Err(error) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({ "error": format!("invalid compute request: {error}") })),
            )
                .into_response();
        }
    };

    match state.engine.verify_escrow(task_id, expected_task_hash).await {
        Ok(true) => {}
        Ok(false) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({
                    "error": "escrow is not a pending payment to this worker for the submitted task hash"
                })),
            )
                .into_response();
        }
        Err(error) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({ "error": format!("escrow verification failed: {error}") })),
            )
                .into_response();
        }
    }

    let (result, result_hash) = execute_task(&request);
    match state.engine.claim_payout(task_id, result_hash).await {
        Ok(claim_tx_hash) => (
            StatusCode::OK,
            Json(ComputeResponse {
                status: "completed".to_string(),
                result,
                task_id: hex_hash(&task_id),
                claim_tx_hash,
            }),
        )
            .into_response(),
        Err(error) => (
            StatusCode::BAD_GATEWAY,
            Json(json!({ "error": format!("claim failed: {error}") })),
        )
            .into_response(),
    }
}
