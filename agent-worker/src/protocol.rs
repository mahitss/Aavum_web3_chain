use serde::{Deserialize, Serialize};

pub const PAYMENT_PROTOCOL: &str = "Nexus402-v1";
pub const PAYMENT_TOKEN: &str = "0x456569838f1d43009A91ebA4263147Afd7233483";
pub const PAYMENT_AMOUNT: &str = "250000";
pub const PAYMENT_AMOUNT_UNITS: u64 = 250_000;
pub const TASK_TIMEOUT_BLOCKS: &str = "50";
pub const CHAIN_ID: u64 = 44005;
pub const RPC_URL: &str = "https://rpc-test.qday.info";

pub const HEADER_PROTOCOL: &str = "x-payment-protocol";
pub const HEADER_TOKEN: &str = "x-payment-token";
pub const HEADER_AMOUNT: &str = "x-payment-amount";
pub const HEADER_RECEIVER: &str = "x-payment-receiver";
pub const HEADER_TASK_HASH: &str = "x-task-hash";
pub const HEADER_TIMEOUT: &str = "x-task-timeout-blocks";
pub const HEADER_ESCROW_TASK_ID: &str = "x-escrow-taskid";

pub const PAYMENT_REQUIRED_ERROR: &str = "Payment required via NexusEscrow on Aevum Testnet";

#[derive(Debug, Clone, Deserialize)]
pub struct ComputeRequest {
    pub prompt: String,
    pub task_type: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct ComputeResponse {
    pub status: String,
    pub result: String,
    pub task_id: String,
    pub claim_tx_hash: String,
}

#[derive(Clone, Copy, Debug)]
pub struct EscrowTaskId(pub [u8; 32]);

pub fn keccak256(bytes: &[u8]) -> [u8; 32] {
    use sha3::{Digest, Keccak256};
    Keccak256::digest(bytes).into()
}

pub fn hex_hash(hash: &[u8; 32]) -> String {
    format!("0x{}", hex::encode(hash))
}

/// Ethereum signed-message digest over `keccak256(abi.encodePacked(taskId, resultHash))`.
pub fn eth_signed_proof_hash(task_id: [u8; 32], result_hash: [u8; 32]) -> [u8; 32] {
    let mut packed = [0u8; 64];
    packed[..32].copy_from_slice(&task_id);
    packed[32..].copy_from_slice(&result_hash);
    let inner = keccak256(&packed);
    let prefix = b"\x19Ethereum Signed Message:\n32";
    let mut message = Vec::with_capacity(prefix.len() + 32);
    message.extend_from_slice(prefix);
    message.extend_from_slice(&inner);
    keccak256(&message)
}

pub fn parse_bytes32(value: &str) -> Option<[u8; 32]> {
    let hex_body = value.strip_prefix("0x").unwrap_or(value);
    let bytes = hex::decode(hex_body).ok()?;
    bytes.try_into().ok()
}
