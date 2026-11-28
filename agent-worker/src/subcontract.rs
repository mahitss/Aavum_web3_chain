use alloy::primitives::Address;
use base64::Engine;
use reqwest::header::HeaderMap;
use serde_json::{json, Value};
use std::str::FromStr;
use std::time::Duration;

use crate::blockchain::BlockchainClient;
use crate::protocol::{
    encode_header, PaymentRequired, PaymentSignature, SubTask, X402Required, DEFAULT_TUSD8_ADDRESS,
    PAYMENT_REQUIRED, PAYMENT_SIGNATURE, PRICE_HOP_CHAIN, PRICE_WEB_BRIEF,
};

pub fn hop_inputs(prompt: &str) -> (String, String) {
    let mut chain = DEFAULT_TUSD8_ADDRESS.to_string();
    let mut web = "https://explorer-test.qday.info".to_string();
    for token in prompt.split_whitespace() {
        if token.starts_with("0x") && token.len() == 42 {
            chain = token.to_string();
        }
        if token.starts_with("https://") || token.starts_with("http://") {
            web = token.trim_end_matches([',', '.']).to_string();
        }
    }
    (chain, web)
}

pub fn synthesize(chain_output: &str, web_output: &str) -> String {
    serde_json::to_string(&json!({
        "service": "deep_due_diligence",
        "margin": crate::protocol::HOP_MARGIN.to_string(),
        "chain": serde_json::from_str::<Value>(chain_output).unwrap_or(Value::String(chain_output.to_string())),
        "web": serde_json::from_str::<Value>(web_output).unwrap_or(Value::String(web_output.to_string())),
    }))
    .unwrap_or_default()
}

pub async fn hire_children(
    chain: &BlockchainClient,
    principal: Address,
    base_url: &str,
    prompt: &str,
) -> Result<(String, String, Vec<SubTask>), String> {
    let (chain_prompt, web_prompt) = hop_inputs(prompt);
    let audit = hire_one(
        chain,
        principal,
        base_url,
        "hop_chain_audit",
        "chain_audit",
        &chain_prompt,
        PRICE_HOP_CHAIN,
    )
    .await?;
    let brief = hire_one(
        chain,
        principal,
        base_url,
        "web_brief",
        "web_brief",
        &web_prompt,
        PRICE_WEB_BRIEF,
    )
    .await?;
    Ok((audit.0, brief.0, vec![audit.1, brief.1]))
}

async fn hire_one(
    chain: &BlockchainClient,
    principal: Address,
    base_url: &str,
    wire_service: &str,
    label: &str,
    prompt: &str,
    amount: u64,
) -> Result<(String, SubTask), String> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(60))
        .build()
        .map_err(|err| err.to_string())?;
    let url = format!("{}/api/v1/compute", base_url.trim_end_matches('/'));
    let body = json!({ "prompt": prompt, "service": wire_service });
    let first = client
        .post(&url)
        .json(&body)
        .send()
        .await
        .map_err(|err| format!("sub-worker challenge failed: {err}"))?;
    if first.status().as_u16() != 402 {
        let text = first.text().await.unwrap_or_default();
        return Err(format!(
            "sub-worker {wire_service} did not challenge payment: {text}"
        ));
    }
    let accepts = decode_accepts(first.headers())?;
    let chosen = accepts
        .into_iter()
        .find(|item| item.scheme == "exact" || item.scheme == "nexus-escrow-v1")
        .ok_or_else(|| format!("{wire_service} offered no exact scheme"))?;
    if chosen.amount != amount.to_string() {
        return Err(format!(
            "{wire_service} quoted {} instead of {amount}",
            chosen.amount
        ));
    }
    let task_hash = decode_hash(&chosen.task_hash)?;
    let pay_to = Address::from_str(chosen.pay_to.trim()).map_err(|err| format!("payTo: {err}"))?;
    let (task_id, lock_tx) = chain
        .lock_escrow(principal, pay_to, amount, task_hash)
        .await
        .map_err(|err| err.to_string())?;
    let signature = PaymentSignature {
        task_id: format!("0x{}", hex::encode(task_id)),
        lock_tx_hash: lock_tx.clone(),
        agent_key: chain.worker_address().to_string(),
        principal: principal.to_string(),
        ..PaymentSignature::default()
    };
    let paid = client
        .post(&url)
        .header(PAYMENT_SIGNATURE.as_str(), encode_header(&signature))
        .json(&body)
        .send()
        .await
        .map_err(|err| format!("sub-worker paid call failed: {err}"))?;
    if !paid.status().is_success() {
        let text = paid.text().await.unwrap_or_default();
        return Err(format!("sub-worker {wire_service} failed: {text}"));
    }
    let payload: Value = paid.json().await.map_err(|err| err.to_string())?;
    let output = payload["result"].as_str().unwrap_or_default().to_string();
    let sub = SubTask {
        service: label.to_string(),
        task_id: payload["task_id"].as_str().unwrap_or_default().to_string(),
        lock_tx_hash: lock_tx,
        claim_tx_hash: payload["claim_tx_hash"]
            .as_str()
            .unwrap_or_default()
            .to_string(),
        amount: amount.to_string(),
    };
    Ok((output, sub))
}

fn decode_accepts(headers: &HeaderMap) -> Result<Vec<PaymentRequired>, String> {
    let encoded = headers
        .get(PAYMENT_REQUIRED.as_str())
        .and_then(|value| value.to_str().ok())
        .ok_or_else(|| "PAYMENT-REQUIRED missing".to_string())?;
    let bytes = base64::engine::general_purpose::STANDARD
        .decode(encoded)
        .map_err(|err| err.to_string())?;
    let envelope: X402Required = serde_json::from_slice(&bytes).map_err(|err| err.to_string())?;
    Ok(envelope.accepts)
}

fn decode_hash(raw: &str) -> Result<[u8; 32], String> {
    let clean = raw.trim().trim_start_matches("0x");
    let bytes = hex::decode(clean).map_err(|err| err.to_string())?;
    if bytes.len() != 32 {
        return Err("task hash is not 32 bytes".to_string());
    }
    let mut out = [0u8; 32];
    out.copy_from_slice(&bytes);
    Ok(out)
}
