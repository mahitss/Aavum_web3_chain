use serde::Deserialize;
use std::time::Duration;

use crate::blockchain::{BlockchainClient, BlockchainError};

#[derive(Debug, Deserialize)]
struct VerifyResponse {
    #[serde(rename = "isValid")]
    is_valid: bool,
    #[serde(default)]
    reason: String,
    #[serde(default)]
    error: String,
}

#[derive(Debug, Deserialize)]
struct SettleResponse {
    settled: bool,
    #[serde(rename = "txHash", default)]
    tx_hash: String,
    #[serde(default)]
    error: String,
}

pub async fn verify_with_facilitator(
    base_url: &str,
    task_id_hex: &str,
    task_hash_hex: &str,
    required_units: u64,
    pay_to: &str,
) -> Result<(), BlockchainError> {
    let body = serde_json::json!({
        "paymentSignature": { "taskId": format!("0x{task_id_hex}") },
        "paymentRequired": {
            "scheme": "exact",
            "network": "eip155:44005",
            "asset": crate::protocol::DEFAULT_TUSD8_ADDRESS,
            "amount": required_units.to_string(),
            "payTo": pay_to,
            "taskHash": task_hash_hex,
            "timeoutBlocks": crate::protocol::TASK_TIMEOUT_BLOCKS
        }
    });
    let response: VerifyResponse = post(base_url, "/facilitator/verify", &body).await?;
    if !response.is_valid {
        let reason = if response.reason.is_empty() {
            response.error
        } else {
            response.reason
        };
        return Err(BlockchainError::RpcError(format!(
            "facilitator rejected escrow: {reason}"
        )));
    }
    Ok(())
}

pub async fn settle_with_facilitator(
    chain: &BlockchainClient,
    base_url: &str,
    task_id_bytes: [u8; 32],
    result_hash: [u8; 32],
    actual_amount: Option<u64>,
) -> Result<String, BlockchainError> {
    let signature = if let Some(amount) = actual_amount {
        chain
            .sign_metered_completion(task_id_bytes, amount, result_hash)
            .await?
    } else {
        chain.sign_completion(task_id_bytes, result_hash).await?
    };
    let body = serde_json::json!({
        "taskId": format!("0x{}", hex::encode(task_id_bytes)),
        "resultHash": format!("0x{}", hex::encode(result_hash)),
        "workerSignature": format!("0x{}", hex::encode(signature)),
        "actualAmount": actual_amount.map(|amount| amount.to_string()).unwrap_or_default(),
    });
    let response: SettleResponse = post(base_url, "/facilitator/settle", &body).await?;
    if !response.settled || response.tx_hash.is_empty() {
        return Err(BlockchainError::RpcError(format!(
            "facilitator settlement failed: {}",
            response.error
        )));
    }
    Ok(response.tx_hash)
}

async fn post<T: for<'de> Deserialize<'de>>(
    base_url: &str,
    path: &str,
    body: &serde_json::Value,
) -> Result<T, BlockchainError> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(90))
        .build()
        .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
    let url = format!("{}{path}", base_url.trim_end_matches('/'));
    let response = client
        .post(url)
        .json(body)
        .send()
        .await
        .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
    let status = response.status();
    let payload: T = response
        .json()
        .await
        .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
    if !status.is_success() {
        return Err(BlockchainError::RpcError(format!(
            "facilitator status {status}"
        )));
    }
    Ok(payload)
}
