use axum::http::HeaderName;
use serde::{Deserialize, Serialize};
use sha3::{Digest, Keccak256};

pub static X_PAYMENT_PROTOCOL: HeaderName = HeaderName::from_static("x-payment-protocol");
pub static X_PAYMENT_TOKEN: HeaderName = HeaderName::from_static("x-payment-token");
pub static X_PAYMENT_AMOUNT: HeaderName = HeaderName::from_static("x-payment-amount");
pub static X_PAYMENT_RECEIVER: HeaderName = HeaderName::from_static("x-payment-receiver");
pub static X_TASK_HASH: HeaderName = HeaderName::from_static("x-task-hash");
pub static X_TASK_TIMEOUT_BLOCKS: HeaderName = HeaderName::from_static("x-task-timeout-blocks");
pub static X_ESCROW_TASK_ID: HeaderName = HeaderName::from_static("x-escrow-task-id");
/// Header the Go orchestrator already sends. Accepted alongside `x-escrow-task-id`.
pub static X_ESCROW_TASK_ID_LEGACY: HeaderName = HeaderName::from_static("x-escrow-taskid");

pub const PROTOCOL_VERSION: &str = "Nexus402-v1";
pub const DEFAULT_TUSD8_ADDRESS: &str = "0x456569838f1d43009A91ebA4263147Afd7233483";
pub const TASK_PRICE_UNITS: u64 = 250_000; // 0.25 tUSD8 (6 decimals)
pub const TASK_TIMEOUT_BLOCKS: u64 = 50;
pub const CHAIN_ID: u64 = 44005;
pub const DEFAULT_RPC_URL: &str = "https://rpc-test.qday.info";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ComputeRequest {
    pub prompt: String,
    pub task_type: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ComputeResponse {
    pub status: String,
    pub result: String,
    pub task_id: String,
    pub task_hash: String,
    pub result_hash: String,
    pub claim_tx_hash: String,
}

#[derive(Debug, Serialize)]
pub struct PaymentRequiredError {
    pub error: String,
    pub message: String,
    pub required_amount: u64,
    pub token: String,
    pub task_hash: String,
}

pub fn calculate_task_hash(payload: &ComputeRequest) -> [u8; 32] {
    let serialized = serde_json::to_vec(payload).unwrap_or_default();
    let mut hasher = Keccak256::new();
    hasher.update(&serialized);
    hasher.finalize().into()
}

pub fn calculate_result_hash(result: &str) -> [u8; 32] {
    let mut hasher = Keccak256::new();
    hasher.update(result.as_bytes());
    hasher.finalize().into()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn task_hash_is_stable_32_bytes() {
        let request = ComputeRequest {
            prompt: "Generate test payload".to_string(),
            task_type: "hash".to_string(),
        };
        let first = calculate_task_hash(&request);
        let second = calculate_task_hash(&request);
        assert_eq!(first, second);
        assert_eq!(first.len(), 32);
        assert_ne!(first, [0u8; 32]);
    }
}
