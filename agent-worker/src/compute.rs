use crate::protocol::{keccak256, ComputeRequest};

/// Deterministic micro-compute. The same request always yields the same result and hash.
pub fn execute_task(request: &ComputeRequest) -> (String, [u8; 32]) {
    let prompt_keccak = format!("0x{}", hex::encode(keccak256(request.prompt.as_bytes())));
    let output = serde_json::json!({
        "protocol": "Nexus402-v1",
        "algorithm": "keccak256",
        "task_type": request.task_type,
        "prompt_keccak": prompt_keccak,
        "prompt_bytes": request.prompt.len(),
    });
    let output = serde_json::to_string(&output).expect("compute result serializes");
    let result_hash = keccak256(output.as_bytes());
    (output, result_hash)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn execute_task_is_stable() {
        let request = ComputeRequest {
            prompt: "Calculate compute hash".to_string(),
            task_type: "hash".to_string(),
        };
        let (first, first_hash) = execute_task(&request);
        let (second, second_hash) = execute_task(&request);
        assert_eq!(first, second);
        assert_eq!(first_hash, second_hash);
        assert_eq!(first_hash, keccak256(first.as_bytes()));
        assert!(first.contains("Nexus402-v1"));
        assert!(first.contains("\"task_type\":\"hash\""));
    }
}
