use axum::body::Body;
use axum::extract::{Request, State};
use axum::http::{HeaderMap, HeaderValue, StatusCode};
use axum::middleware::Next;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde_json::json;

use crate::protocol::{
    self, EscrowTaskId, HEADER_AMOUNT, HEADER_ESCROW_TASK_ID, HEADER_PROTOCOL, HEADER_RECEIVER,
    HEADER_TASK_HASH, HEADER_TIMEOUT, HEADER_TOKEN, PAYMENT_AMOUNT, PAYMENT_PROTOCOL,
    PAYMENT_REQUIRED_ERROR, PAYMENT_TOKEN, TASK_TIMEOUT_BLOCKS,
};

#[derive(Clone)]
pub struct PaywallState {
    pub receiver: String,
}

pub async fn require_escrow(
    State(paywall): State<PaywallState>,
    request: Request,
    next: Next,
) -> Response {
    let (parts, body) = request.into_parts();
    let bytes = match axum::body::to_bytes(body, 1024 * 1024).await {
        Ok(bytes) => bytes,
        Err(error) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({ "error": format!("unreadable body: {error}") })),
            )
                .into_response();
        }
    };

    let task_header = parts
        .headers
        .get(HEADER_ESCROW_TASK_ID)
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty());

    let Some(raw_task_id) = task_header else {
        let task_hash = protocol::keccak256(&bytes);
        return payment_required(&paywall.receiver, &task_hash);
    };

    let Some(task_id) = protocol::parse_bytes32(raw_task_id) else {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": "X-Escrow-TaskId must be a 32-byte hex value" })),
        )
            .into_response();
    };

    let mut request = Request::from_parts(parts, Body::from(bytes));
    request.extensions_mut().insert(EscrowTaskId(task_id));
    next.run(request).await
}

fn payment_required(receiver: &str, task_hash: &[u8; 32]) -> Response {
    let mut headers = HeaderMap::new();
    insert(&mut headers, HEADER_PROTOCOL, PAYMENT_PROTOCOL);
    insert(&mut headers, HEADER_TOKEN, PAYMENT_TOKEN);
    insert(&mut headers, HEADER_AMOUNT, PAYMENT_AMOUNT);
    insert(&mut headers, HEADER_RECEIVER, receiver);
    insert(&mut headers, HEADER_TASK_HASH, &protocol::hex_hash(task_hash));
    insert(&mut headers, HEADER_TIMEOUT, TASK_TIMEOUT_BLOCKS);
    (
        StatusCode::PAYMENT_REQUIRED,
        headers,
        Json(json!({ "error": PAYMENT_REQUIRED_ERROR })),
    )
        .into_response()
}

fn insert(headers: &mut HeaderMap, name: &'static str, value: &str) {
    if let Ok(value) = HeaderValue::from_str(value) {
        headers.insert(name, value);
    }
}
