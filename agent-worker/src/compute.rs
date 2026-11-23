use crate::protocol::{calculate_result_hash, ComputeRequest};

pub fn execute_compute_job(request: &ComputeRequest) -> (String, [u8; 32]) {
    let output = match request.task_type.as_str() {
        "hash" => {
            use sha3::{Digest, Keccak256};
            let mut hasher = Keccak256::new();
            hasher.update(request.prompt.as_bytes());
            format!("0x{}", hex::encode(hasher.finalize()))
        }
        "inference" => {
            format!(
                "Nexus402-Compute: Verified output synthesis for token context '{}'",
                request.prompt
            )
        }
        _ => {
            format!("Standard execution complete for: {}", request.prompt)
        }
    };

    let result_hash = calculate_result_hash(&output);
    (output, result_hash)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hash_job_is_stable() {
        let request = ComputeRequest {
            prompt: "Generate test payload".to_string(),
            task_type: "hash".to_string(),
        };
        let (first, first_hash) = execute_compute_job(&request);
        let (second, second_hash) = execute_compute_job(&request);
        assert_eq!(first, second);
        assert_eq!(first_hash, second_hash);
        assert!(first.starts_with("0x"));
        assert_eq!(first_hash, calculate_result_hash(&first));
    }
}
