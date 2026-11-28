use alloy::primitives::{Address, Signature, B256, U256};
use axum::extract::{Path, State};
use axum::http::{HeaderMap, StatusCode};
use axum::response::IntoResponse;
use axum::Json;
use sha3::{Digest, Keccak256};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use crate::compute;
use crate::protocol::{
    hash_bytes, service_price, ComputeRequest, ComputeResponse, PaymentSignature,
    EXPLORER_TX_PREFIX,
};

#[derive(Clone)]
pub struct StoredVoucher {
    pub cumulative: u64,
    pub nonce: u64,
    pub valid_before: u64,
    pub signature: Vec<u8>,
}

#[derive(Clone)]
pub struct OpenTab {
    pub deposit: u64,
    pub agent: Address,
    pub last_cumulative: u64,
    pub last_nonce: u64,
    pub calls: u64,
    pub latest: Option<StoredVoucher>,
}

pub type TabBook = Mutex<HashMap<[u8; 32], OpenTab>>;

pub fn voucher_digest(
    escrow: Address,
    tab_id: [u8; 32],
    cumulative: u64,
    nonce: u64,
    valid_before: u64,
) -> [u8; 32] {
    let domain_type = keccak(
        b"EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)",
    );
    let name = keccak(b"Nexus402 Protocol");
    let version = keccak(b"1");
    let domain = keccak(
        &[
            domain_type.as_slice(),
            name.as_slice(),
            version.as_slice(),
            &word_u64(crate::protocol::CHAIN_ID),
            &word_address(escrow),
        ]
        .concat(),
    );
    let voucher_type = keccak(
        b"PaymentVoucher(bytes32 tabId,uint256 cumulativeAmount,uint256 nonce,uint256 validBefore)",
    );
    let body = keccak(
        &[
            voucher_type.as_slice(),
            tab_id.as_slice(),
            &word_u64(cumulative),
            &word_u64(nonce),
            &word_u64(valid_before),
        ]
        .concat(),
    );
    let mut prefixed = vec![0x19, 0x01];
    prefixed.extend(domain);
    prefixed.extend(body);
    keccak(&prefixed)
}

pub fn recover_signer(digest: [u8; 32], signature: &[u8]) -> Result<Address, String> {
    if signature.len() != 65 {
        return Err("voucher signature must be 65 bytes".into());
    }
    let mut normalized = signature.to_vec();
    if normalized[64] >= 27 {
        normalized[64] -= 27;
    }
    let parsed = Signature::try_from(normalized.as_slice()).map_err(|err| err.to_string())?;
    parsed
        .recover_address_from_prehash(&B256::from(digest))
        .map_err(|err| err.to_string())
}

pub async fn accept(
    state: Arc<crate::AppState>,
    headers: &HeaderMap,
    payload: &ComputeRequest,
    service: &str,
    payment: &PaymentSignature,
) -> axum::response::Response {
    let tab_id = match decode_word(&payment.tab_id) {
        Ok(value) => value,
        Err(err) => return bad(err),
    };
    let cumulative = match payment.cumulative_amount.parse::<u64>() {
        Ok(value) => value,
        Err(_) => return bad("invalid cumulativeAmount".into()),
    };
    let signature = match hex::decode(payment.agent_signature.trim().trim_start_matches("0x")) {
        Ok(value) => value,
        Err(_) => return bad("invalid agentSignature".into()),
    };
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|value| value.as_secs())
        .unwrap_or(0);
    if payment.valid_before < now {
        return bad("voucher expired".into());
    }
    let price = service_price(service);
    let digest = voucher_digest(
        state.blockchain.escrow_address(),
        tab_id,
        cumulative,
        payment.nonce,
        payment.valid_before,
    );
    let signer = match recover_signer(digest, &signature) {
        Ok(address) => address,
        Err(err) => return bad(err),
    };

    let needs_chain_read = {
        let book = state.tabs.lock().unwrap_or_else(|err| err.into_inner());
        !book.contains_key(&tab_id)
    };
    if needs_chain_read {
        let loaded = match state.blockchain.load_tab(tab_id).await {
            Ok(tab) => tab,
            Err(err) => return bad(err.to_string()),
        };
        if !loaded.open {
            return bad("tab is closed".into());
        }
        if loaded.worker != state.blockchain.worker_address() {
            return bad("tab worker does not match this node".into());
        }
        let mut book = state.tabs.lock().unwrap_or_else(|err| err.into_inner());
        book.entry(tab_id).or_insert(OpenTab {
            deposit: loaded.deposit,
            agent: loaded.agent,
            last_cumulative: 0,
            last_nonce: loaded.last_nonce,
            calls: 0,
            latest: None,
        });
    }
    let calls = {
        let mut book = state.tabs.lock().unwrap_or_else(|err| err.into_inner());
        let Some(tab) = book.get_mut(&tab_id) else {
            return bad("tab is closed".into());
        };
        if signer != tab.agent {
            return bad("voucher signer is not the tab agent".into());
        }
        if cumulative > tab.deposit {
            return bad("voucher exceeds the tab deposit".into());
        }
        if payment.nonce <= tab.last_nonce {
            return bad("stale voucher nonce".into());
        }
        if cumulative < tab.last_cumulative.saturating_add(price) {
            return bad("voucher does not cover the service price".into());
        }
        let mut stored = signature;
        if stored[64] < 27 {
            stored[64] += 27;
        }
        tab.last_cumulative = cumulative;
        tab.last_nonce = payment.nonce;
        tab.calls += 1;
        tab.latest = Some(StoredVoucher {
            cumulative,
            nonce: payment.nonce,
            valid_before: payment.valid_before,
            signature: stored,
        });
        tab.calls
    };

    let mut request = payload.clone();
    request.service = service.to_string();
    let output = match compute::execute_service(&request, &state.blockchain).await {
        Ok((output, _)) => output,
        Err(err) => return bad(err),
    };
    let defer = headers
        .get("x-defer-voucher-flush")
        .and_then(|value| value.to_str().ok())
        .map(|value| value == "1" || value.eq_ignore_ascii_case("true"))
        .unwrap_or(false);
    let flush_every = std::env::var("VOUCHER_FLUSH_EVERY")
        .ok()
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(5);
    if !defer && flush_every > 0 && calls % flush_every == 0 {
        let background = Arc::clone(&state);
        let tab_copy = tab_id;
        tokio::spawn(async move {
            let _ = settle_tab(&background, tab_copy).await;
        });
    }
    let result_hash = hash_bytes(output.as_bytes());
    let body = ComputeResponse {
        status: "VOUCHER_ACCEPTED".to_string(),
        result: output,
        task_id: String::new(),
        task_hash: String::new(),
        result_hash: format!("0x{}", hex::encode(result_hash)),
        claim_tx_hash: String::new(),
        ceiling: None,
        actual_amount: Some(cumulative.to_string()),
        refunded_change: None,
        breakdown: None,
        provenance: None,
    };
    (StatusCode::OK, Json(body)).into_response()
}

pub async fn settle_handler(
    State(state): State<Arc<crate::AppState>>,
    Path(tab_id): Path<String>,
) -> impl IntoResponse {
    let tab_id = match decode_word(&tab_id) {
        Ok(value) => value,
        Err(err) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(serde_json::json!({"error": err})),
            )
                .into_response()
        }
    };
    match settle_tab(&state, tab_id).await {
        Ok(tx_hash) => (
            StatusCode::OK,
            Json(serde_json::json!({
                "settled": true,
                "txHash": tx_hash,
                "explorerUrl": format!("{EXPLORER_TX_PREFIX}{tx_hash}")
            })),
        )
            .into_response(),
        Err(err) => (
            StatusCode::BAD_GATEWAY,
            Json(serde_json::json!({"settled": false, "error": err})),
        )
            .into_response(),
    }
}

async fn settle_tab(state: &Arc<crate::AppState>, tab_id: [u8; 32]) -> Result<String, String> {
    let voucher = {
        let book = state.tabs.lock().unwrap_or_else(|err| err.into_inner());
        book.get(&tab_id)
            .and_then(|tab| tab.latest.clone())
            .ok_or_else(|| "no voucher is waiting for this tab".to_string())?
    };
    let tx = state
        .blockchain
        .settle_voucher(
            tab_id,
            voucher.cumulative,
            voucher.nonce,
            voucher.valid_before,
            voucher.signature,
        )
        .await
        .map_err(|err| err.to_string())?;
    let mut book = state.tabs.lock().unwrap_or_else(|err| err.into_inner());
    book.remove(&tab_id);
    Ok(tx)
}

fn bad(error: String) -> axum::response::Response {
    (
        StatusCode::PAYMENT_REQUIRED,
        Json(serde_json::json!({"error": error})),
    )
        .into_response()
}

fn decode_word(raw: &str) -> Result<[u8; 32], String> {
    let bytes = hex::decode(raw.trim().trim_start_matches("0x")).map_err(|err| err.to_string())?;
    if bytes.len() != 32 {
        return Err("expected 32-byte hex".into());
    }
    let mut out = [0u8; 32];
    out.copy_from_slice(&bytes);
    Ok(out)
}

fn keccak(bytes: &[u8]) -> [u8; 32] {
    let mut hasher = Keccak256::new();
    hasher.update(bytes);
    hasher.finalize().into()
}

fn word_u64(value: u64) -> [u8; 32] {
    U256::from(value).to_be_bytes()
}

fn word_address(address: Address) -> [u8; 32] {
    let mut word = [0u8; 32];
    word[12..].copy_from_slice(address.as_slice());
    word
}

#[cfg(test)]
mod tests {
    use super::*;
    use alloy::signers::local::PrivateKeySigner;
    use alloy::signers::Signer;

    #[tokio::test]
    async fn voucher_digest_recovers_the_signing_agent() {
        let signer = PrivateKeySigner::random();
        let escrow = Address::repeat_byte(0x11);
        let tab = [9u8; 32];
        let digest = voucher_digest(escrow, tab, 50_000, 1, 1_900_000_000);
        let signature = signer.sign_hash(&B256::from(digest)).await.unwrap();
        let recovered = recover_signer(digest, &signature.as_bytes()).unwrap();
        assert_eq!(recovered, signer.address());
        assert_ne!(
            voucher_digest(escrow, tab, 100_000, 2, 1_900_000_000),
            digest
        );
    }
}
