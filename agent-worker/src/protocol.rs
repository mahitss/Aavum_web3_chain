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
pub const PRICE_HOP_CHAIN: u64 = 250_000;
pub const PRICE_DEEP_DUE_DILIGENCE: u64 = 600_000;
pub const HOP_MARGIN: u64 = 200_000;
pub const PRICE_MICRO_CALL: u64 = 50_000;
pub const PRICE_UPTO_CEILING: u64 = 500_000;
pub const METER_BASE_UNITS: u64 = 50_000;
pub const METER_PER_TOKEN: u64 = 200;
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
    #[serde(default)]
    pub simulate_garbage: bool,
    #[serde(default)]
    pub scheme: String,
    #[serde(default)]
    pub batch: Vec<String>,
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

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PaymentSignature {
    #[serde(rename = "taskId", default)]
    pub task_id: String,
    #[serde(rename = "lockTxHash", default)]
    pub lock_tx_hash: String,
    #[serde(rename = "agentKey", default)]
    pub agent_key: String,
    #[serde(default)]
    pub principal: String,
    #[serde(default)]
    pub scheme: String,
    #[serde(rename = "tabId", default)]
    pub tab_id: String,
    #[serde(rename = "cumulativeAmount", default)]
    pub cumulative_amount: String,
    #[serde(default)]
    pub nonce: u64,
    #[serde(rename = "validBefore", default)]
    pub valid_before: u64,
    #[serde(rename = "agentSignature", default)]
    pub agent_signature: String,
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
    #[serde(rename = "actualAmount", skip_serializing_if = "Option::is_none")]
    pub actual_amount: Option<String>,
    #[serde(rename = "refundedChange", skip_serializing_if = "Option::is_none")]
    pub refunded_change: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ComputeResponse {
    pub status: String,
    pub result: String,
    pub task_id: String,
    pub task_hash: String,
    pub result_hash: String,
    pub claim_tx_hash: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ceiling: Option<String>,
    #[serde(rename = "actualAmount", skip_serializing_if = "Option::is_none")]
    pub actual_amount: Option<String>,
    #[serde(rename = "refundedChange", skip_serializing_if = "Option::is_none")]
    pub refunded_change: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub breakdown: Option<Vec<MeteredCall>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provenance: Option<Provenance>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Provenance {
    #[serde(rename = "parentTaskId")]
    pub parent_task_id: String,
    #[serde(rename = "parentAmount")]
    pub parent_amount: String,
    pub margin: String,
    #[serde(rename = "subTasks")]
    pub sub_tasks: Vec<SubTask>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SubTask {
    pub service: String,
    #[serde(rename = "taskId")]
    pub task_id: String,
    #[serde(rename = "lockTxHash")]
    pub lock_tx_hash: String,
    #[serde(rename = "claimTxHash")]
    pub claim_tx_hash: String,
    pub amount: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MeteredCall {
    pub prompt: String,
    pub tokens: u64,
    pub amount: String,
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
        "hop_chain_audit" => Ok("hop_chain_audit"),
        "deep_due_diligence" => Ok("deep_due_diligence"),
        "micro_call" => Ok("micro_call"),
        "ai_synthesis" | "hash" | "inference" | "" => Ok("ai_synthesis"),
        other => Err(format!("unknown service: {other}")),
    }
}

pub fn meter_usage(output: &str) -> u64 {
    let tokens = output.chars().count().div_ceil(4).max(1) as u64;
    METER_BASE_UNITS
        .saturating_add(METER_PER_TOKEN.saturating_mul(tokens))
        .min(PRICE_UPTO_CEILING)
}

pub fn token_count(output: &str) -> u64 {
    output.chars().count().div_ceil(4).max(1) as u64
}

pub fn service_price(service: &str) -> u64 {
    match service {
        "web_brief" => PRICE_WEB_BRIEF,
        "chain_audit" => PRICE_CHAIN_AUDIT,
        "hop_chain_audit" => PRICE_HOP_CHAIN,
        "deep_due_diligence" => PRICE_DEEP_DUE_DILIGENCE,
        "micro_call" => PRICE_MICRO_CALL,
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

pub fn hash_bytes(bytes: &[u8]) -> [u8; 32] {
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
            simulate_garbage: false,
            scheme: String::new(),
            batch: Vec::new(),
        };
        let first = calculate_task_hash(&request);
        let second = calculate_task_hash(&request);
        assert_eq!(first, second);
        assert_eq!(first.len(), 32);
        assert_ne!(first, [0u8; 32]);
    }

    #[test]
    fn deep_due_diligence_keeps_a_point_two_margin() {
        assert_eq!(
            PRICE_DEEP_DUE_DILIGENCE,
            PRICE_HOP_CHAIN + PRICE_WEB_BRIEF + HOP_MARGIN
        );
    }

    #[test]
    fn meter_includes_base_fee_and_caps_at_ceiling() {
        assert_eq!(meter_usage("abcd"), METER_BASE_UNITS + METER_PER_TOKEN);
        assert_eq!(meter_usage(&"a".repeat(20_000)), PRICE_UPTO_CEILING);
    }
}
