use axum::http::HeaderName;
use base64::engine::general_purpose::STANDARD;
use base64::Engine;
use serde::{Deserialize, Serialize};
use sha3::{Digest, Keccak256};

pub static PAYMENT_REQUIRED: HeaderName = HeaderName::from_static("payment-required");
pub static PAYMENT_SIGNATURE: HeaderName = HeaderName::from_static("payment-signature");
pub static PAYMENT_RESPONSE: HeaderName = HeaderName::from_static("payment-response");
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
pub const PRICE_WEB_BRIEF: u64 = 150_000;
pub const PRICE_AI_SYNTHESIS: u64 = 250_000;
pub const PRICE_CHAIN_AUDIT: u64 = 500_000;
pub const TASK_TIMEOUT_BLOCKS: u64 = 50;
pub const EXPLORER_TX_PREFIX: &str = "https://explorer-test.qday.info/tx/";
pub const CHAIN_ID: u64 = 44005;
pub const DEFAULT_RPC_URL: &str = "https://rpc-test.qday.info";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ComputeRequest {
    pub prompt: String,
    #[serde(default)]
    pub service: String,
    #[serde(default)]
    pub task_type: String,
    #[serde(default)]
    pub simulate_timeout: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaymentRequired {
    pub scheme: String,
    pub network: String,
    pub asset: String,
    pub amount: String,
    #[serde(rename = "payTo")]
    pub pay_to: String,
    #[serde(rename = "taskHash")]
    pub task_hash: String,
    #[serde(rename = "timeoutBlocks")]
    pub timeout_blocks: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaymentSignature {
    #[serde(rename = "taskId")]
    pub task_id: String,
    #[serde(rename = "lockTxHash")]
    pub lock_tx_hash: String,
    #[serde(rename = "agentKey")]
    pub agent_key: String,
    pub principal: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct X402Required {
    #[serde(rename = "x402Version")]
    pub x402_version: u8,
    pub accepts: Vec<PaymentRequired>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaymentResponseHeader {
    #[serde(rename = "x402Version")]
    pub x402_version: u8,
    pub status: String,
    #[serde(rename = "claimTxHash")]
    pub claim_tx_hash: String,
    #[serde(rename = "resultHash")]
    pub result_hash: String,
    #[serde(rename = "explorerUrl")]
    pub explorer_url: String,
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

pub fn resolve_service(payload: &ComputeRequest) -> Result<&'static str, String> {
    let raw = if payload.service.trim().is_empty() {
        payload.task_type.trim()
    } else {
        payload.service.trim()
    };
    match raw {
        "web_brief" => Ok("web_brief"),
        "chain_audit" => Ok("chain_audit"),
        "ai_synthesis" | "hash" | "inference" | "" => Ok("ai_synthesis"),
        other => Err(format!("unknown service: {other}")),
    }
}

pub fn service_price(service: &str) -> u64 {
    match service {
        "web_brief" => PRICE_WEB_BRIEF,
        "chain_audit" => PRICE_CHAIN_AUDIT,
        _ => PRICE_AI_SYNTHESIS,
    }
}

pub fn canonical_task_bytes(prompt: &str, service: &str) -> Vec<u8> {
    serde_json::to_vec(&serde_json::json!({
        "prompt": prompt,
        "service": service,
    }))
    .unwrap_or_default()
}

pub fn calculate_task_hash(payload: &ComputeRequest) -> [u8; 32] {
    let service = resolve_service(payload).unwrap_or("ai_synthesis");
    hash_bytes(&canonical_task_bytes(&payload.prompt, service))
}

pub fn encode_header(value: &impl Serialize) -> String {
    let bytes = serde_json::to_vec(value).unwrap_or_default();
    STANDARD.encode(bytes)
}

pub fn decode_header<T: for<'de> Deserialize<'de>>(value: &str) -> Result<T, String> {
    let bytes = STANDARD
        .decode(value.trim().as_bytes())
        .map_err(|err| err.to_string())?;
    serde_json::from_slice(&bytes).map_err(|err| err.to_string())
}

fn hash_bytes(bytes: &[u8]) -> [u8; 32] {
    let mut hasher = Keccak256::new();
    hasher.update(bytes);
    hasher.finalize().into()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn task_hash_is_stable_32_bytes() {
        let request = ComputeRequest {
            prompt: "Generate test payload".to_string(),
            service: String::new(),
            task_type: "hash".to_string(),
            simulate_timeout: false,
        };
        let first = calculate_task_hash(&request);
        let second = calculate_task_hash(&request);
        assert_eq!(first, second);
        assert_eq!(first.len(), 32);
        assert_ne!(first, [0u8; 32]);
    }
}
