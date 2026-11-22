use alloy::primitives::{keccak256, B256};
use serde_json::{json, Value};

/// Deterministic compute unit. The same payload always yields the same bytes.
pub fn execute(payload: &[u8]) -> Vec<u8> {
    let digest = keccak256(payload);
    let value = json!({
        "protocol": "Nexus402-v1",
        "algorithm": "keccak256",
        "inputKeccak": format!("{digest}"),
        "bytes": payload.len(),
    });
    serde_json::to_vec(&value).expect("canonical result serializes")
}

pub fn result_hash(result: &[u8]) -> B256 {
    keccak256(result)
}

pub fn result_value(result: &[u8]) -> Value {
    serde_json::from_slice(result).unwrap_or_else(|_| json!({ "raw": hex_encode(result) }))
}

fn hex_encode(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

/// Digest the worker signs: keccak256(abi.encodePacked(taskId, resultHash)).
pub fn proof_digest(task_id: B256, result_hash: B256) -> B256 {
    let mut raw = [0u8; 64];
    raw[..32].copy_from_slice(task_id.as_slice());
    raw[32..].copy_from_slice(result_hash.as_slice());
    keccak256(raw)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn compute_is_stable() {
        let payload = br#"{"job":"keccak256","n":1}"#;
        assert_eq!(execute(payload), execute(payload));
        let result = execute(payload);
        let parsed: serde_json::Value = serde_json::from_slice(&result).unwrap();
        assert_eq!(parsed["protocol"], "Nexus402-v1");
        assert_eq!(parsed["algorithm"], "keccak256");
        assert_eq!(result_hash(&result), keccak256(&result));
    }

    #[test]
    fn proof_digest_packs_task_and_result() {
        let task = B256::from([0x11u8; 32]);
        let result = B256::from([0x22u8; 32]);
        let mut raw = [0u8; 64];
        raw[..32].copy_from_slice(&[0x11u8; 32]);
        raw[32..].copy_from_slice(&[0x22u8; 32]);
        assert_eq!(proof_digest(task, result), keccak256(raw));
    }
}
