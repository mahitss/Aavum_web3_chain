use std::time::Duration;

use alloy::primitives::{Address, Bytes, B256, U256};
use alloy::providers::Provider;
use alloy::signers::local::PrivateKeySigner;
use alloy::signers::Signer;
use alloy::sol;
use anyhow::{anyhow, Context};

use crate::compute::{execute, proof_digest, result_hash, result_value};

sol! {
    #[sol(rpc)]
    interface INexusEscrow {
        function getTask(bytes32 taskId) external view returns (
            bytes32 id,
            address principal,
            address agentKey,
            address worker,
            uint256 amount,
            uint256 deadline,
            bytes32 taskHash,
            bytes32 resultHash,
            uint8 status
        );

        function completeTaskWithProof(
            bytes32 taskId,
            bytes32 resultHash,
            bytes signature
        ) external;
    }
}

const PENDING: u8 = 1;

pub struct Claim {
    pub result: serde_json::Value,
    pub result_hash: B256,
    pub claim_tx: B256,
}

pub async fn verify_and_claim<P>(
    provider: &P,
    signer: &PrivateKeySigner,
    escrow: Address,
    worker: Address,
    required_amount: U256,
    task_id: B256,
    payload: &[u8],
    payload_hash: B256,
) -> anyhow::Result<Claim>
where
    P: Provider + Clone,
{
    let contract = INexusEscrow::new(escrow, provider);
    let task = contract
        .getTask(task_id)
        .call()
        .await
        .context("getTask failed")?;

    if task.status == 0 {
        return Err(anyhow!("task not found"));
    }
    if task.status != PENDING {
        return Err(anyhow!("task is not pending"));
    }
    if task.worker != worker {
        return Err(anyhow!("escrow worker does not match this agent"));
    }
    if task.amount != required_amount {
        return Err(anyhow!(
            "escrow amount {} does not match required fee {}",
            task.amount,
            required_amount
        ));
    }
    if task.taskHash != payload_hash {
        return Err(anyhow!("escrow task hash does not match this payload"));
    }

    let block = provider.get_block_number().await.context("block number")?;
    if block > task.deadline.to::<u64>() {
        return Err(anyhow!("task deadline has passed"));
    }

    let result = execute(payload);
    let hashed = result_hash(&result);
    let digest = proof_digest(task_id, hashed);
    let signature = signer.sign_hash(&digest).await.context("sign proof")?;
    let mut raw = signature.as_bytes();
    if raw[64] < 27 {
        raw[64] += 27;
    }

    let pending = contract
        .completeTaskWithProof(task_id, hashed, Bytes::copy_from_slice(&raw))
        .send()
        .await
        .context("completeTaskWithProof failed")?;
    let receipt = tokio::time::timeout(Duration::from_secs(75), pending.get_receipt())
        .await
        .context("timed out waiting for claim receipt")?
        .context("claim receipt")?;
    if !receipt.status() {
        return Err(anyhow!("claim transaction reverted"));
    }

    Ok(Claim {
        result: result_value(&result),
        result_hash: hashed,
        claim_tx: receipt.transaction_hash,
    })
}
