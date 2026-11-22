use alloy::primitives::{Address, B256, U256};
use axum::http::{HeaderMap, HeaderValue, StatusCode};
use axum::response::{IntoResponse, Response};

pub const PROTOCOL: &str = "Nexus402-v1";
pub const TUSD8: &str = "0x456569838f1d43009A91ebA4263147Afd7233483";

pub struct PaymentChallenge {
    pub receiver: Address,
    pub amount: U256,
    pub task_hash: B256,
    pub timeout_blocks: u64,
}

pub fn payment_required(challenge: &PaymentChallenge) -> Response {
    let mut headers = HeaderMap::new();
    insert(&mut headers, "x-payment-protocol", PROTOCOL);
    insert(&mut headers, "x-payment-token", TUSD8);
    insert(&mut headers, "x-payment-amount", &challenge.amount.to_string());
    insert(&mut headers, "x-payment-receiver", &format!("{}", challenge.receiver));
    insert(&mut headers, "x-task-hash", &format!("{}", challenge.task_hash));
    insert(
        &mut headers,
        "x-task-timeout-blocks",
        &challenge.timeout_blocks.to_string(),
    );
    insert(
        &mut headers,
        "access-control-expose-headers",
        "X-Payment-Protocol, X-Payment-Token, X-Payment-Amount, X-Payment-Receiver, X-Task-Hash, X-Task-Timeout-Blocks",
    );
    (StatusCode::PAYMENT_REQUIRED, headers, "Payment Required").into_response()
}

fn insert(headers: &mut HeaderMap, name: &'static str, value: &str) {
    if let Ok(parsed) = HeaderValue::from_str(value) {
        headers.insert(name, parsed);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use alloy::primitives::keccak256;
    use axum::body::to_bytes;
    use axum::http::StatusCode;

    #[tokio::test]
    async fn challenge_advertises_nexus402_terms() {
        let body = br#"{"job":"keccak256"}"#;
        let response = payment_required(&PaymentChallenge {
            receiver: Address::from([0x22u8; 20]),
            amount: U256::from(250_000u64),
            task_hash: keccak256(body),
            timeout_blocks: 50,
        });
        assert_eq!(response.status(), StatusCode::PAYMENT_REQUIRED);
        let headers = response.headers();
        assert_eq!(headers.get("x-payment-protocol").unwrap(), "Nexus402-v1");
        assert_eq!(
            headers.get("x-payment-token").unwrap(),
            "0x456569838f1d43009A91ebA4263147Afd7233483"
        );
        assert_eq!(headers.get("x-payment-amount").unwrap(), "250000");
        assert_eq!(headers.get("x-task-timeout-blocks").unwrap(), "50");
        assert_eq!(
            headers.get("x-task-hash").unwrap().to_str().unwrap(),
            format!("{}", keccak256(body))
        );
        let bytes = to_bytes(response.into_body(), usize::MAX).await.unwrap();
        assert_eq!(&bytes[..], b"Payment Required");
    }
}
