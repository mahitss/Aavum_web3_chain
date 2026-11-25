use alloy::network::{Ethereum, EthereumWallet};
use alloy::primitives::{Address, Bytes, B256, U256};
use alloy::providers::fillers::{
    ChainIdFiller, FillProvider, GasFiller, JoinFill, NonceFiller, WalletFiller,
};
use alloy::providers::{Identity, ProviderBuilder, RootProvider};
use alloy::signers::local::PrivateKeySigner;
use alloy::signers::Signer;
use alloy::sol;
use sha3::{Digest, Keccak256};
use std::str::FromStr;
use thiserror::Error;

use alloy::providers::Provider;

use crate::protocol::{CHAIN_ID, DEFAULT_TUSD8_ADDRESS};

sol! {
    #[sol(rpc)]
    interface INexusEscrow {
        struct TaskEscrow {
            bytes32 taskId;
            address principal;
            address agentKey;
            address worker;
            uint256 amount;
            uint256 deadline;
            bytes32 taskHash;
            bytes32 resultHash;
            uint8 status;
        }

        function getTask(bytes32 taskId) external view returns (TaskEscrow memory);

        function completeTaskWithProof(
            bytes32 taskId,
            bytes32 resultHash,
            bytes calldata workerSignature
        ) external;
    }

    #[sol(rpc)]
    interface IErc20Balance {
        function balanceOf(address account) external view returns (uint256);
    }
}

type HttpTransport = alloy::transports::http::ReqwestTransport;
type AevumFiller = JoinFill<
    JoinFill<JoinFill<JoinFill<Identity, GasFiller>, NonceFiller>, ChainIdFiller>,
    WalletFiller<EthereumWallet>,
>;
type AevumProvider =
    FillProvider<AevumFiller, RootProvider<HttpTransport>, HttpTransport, Ethereum>;

#[derive(Error, Debug)]
pub enum BlockchainError {
    #[error("Invalid task ID hex format")]
    InvalidTaskId,
    #[error("Task not found or status not PENDING (status code {0})")]
    InvalidTaskStatus(u8),
    #[error("Assigned worker {0} does not match node worker address {1}")]
    WorkerMismatch(Address, Address),
    #[error("Locked escrow amount {0} is below required fee {1}")]
    InsufficientEscrowAmount(U256, U256),
    #[error("Task hash mismatch on-chain")]
    TaskHashMismatch,
    #[error("RPC or contract error: {0}")]
    RpcError(String),
}

#[derive(Clone)]
pub struct BlockchainClient {
    signer: PrivateKeySigner,
    worker_address: Address,
    escrow_address: Address,
    contract: INexusEscrow::INexusEscrowInstance<HttpTransport, AevumProvider>,
}

impl BlockchainClient {
    pub fn new(
        rpc_url: &str,
        private_key_hex: &str,
        escrow_address_hex: &str,
    ) -> Result<Self, Box<dyn std::error::Error>> {
        let mut signer = PrivateKeySigner::from_str(private_key_hex)?;
        signer.set_chain_id(Some(CHAIN_ID));
        let worker_address = signer.address();
        let escrow_address = Address::from_str(escrow_address_hex)?;
        let wallet = EthereumWallet::from(signer.clone());
        let url = rpc_url.parse()?;
        let provider = ProviderBuilder::new()
            .with_recommended_fillers()
            .wallet(wallet)
            .on_http(url);

        Ok(Self {
            signer,
            worker_address,
            escrow_address,
            contract: INexusEscrow::new(escrow_address, provider),
        })
    }

    pub fn worker_address(&self) -> Address {
        self.worker_address
    }

    pub fn escrow_address(&self) -> Address {
        self.escrow_address
    }

    pub fn parse_task_id(task_id_hex: &str) -> Result<[u8; 32], BlockchainError> {
        let mut task_id_bytes = [0u8; 32];
        hex::decode_to_slice(task_id_hex, &mut task_id_bytes)
            .map_err(|_| BlockchainError::InvalidTaskId)?;
        Ok(task_id_bytes)
    }

    pub async fn verify_escrow(
        &self,
        task_id_bytes: [u8; 32],
        expected_task_hash: [u8; 32],
        required_units: u64,
    ) -> Result<(), BlockchainError> {
        let task_id = B256::from(task_id_bytes);
        let returned = self
            .contract
            .getTask(task_id)
            .call()
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
        let task = returned._0;

        if task.status != 1 {
            return Err(BlockchainError::InvalidTaskStatus(task.status));
        }
        if task.worker != self.worker_address {
            return Err(BlockchainError::WorkerMismatch(
                task.worker,
                self.worker_address,
            ));
        }
        let required_amount = U256::from(required_units);
        if task.amount < required_amount {
            return Err(BlockchainError::InsufficientEscrowAmount(
                task.amount,
                required_amount,
            ));
        }
        if task.taskHash != B256::from(expected_task_hash) {
            return Err(BlockchainError::TaskHashMismatch);
        }
        Ok(())
    }

    pub async fn claim_payout(
        &self,
        task_id_bytes: [u8; 32],
        result_hash_bytes: [u8; 32],
    ) -> Result<String, BlockchainError> {
        let mut hasher = Keccak256::new();
        hasher.update(task_id_bytes);
        hasher.update(result_hash_bytes);
        let raw_digest: [u8; 32] = hasher.finalize().into();

        let signature = self
            .signer
            .sign_message(&raw_digest)
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
        let mut signature_bytes = signature.as_bytes();
        if signature_bytes[64] < 27 {
            signature_bytes[64] += 27;
        }

        let call = self.contract.completeTaskWithProof(
            B256::from(task_id_bytes),
            B256::from(result_hash_bytes),
            Bytes::from(signature_bytes.to_vec()),
        );
        let pending_tx = call
            .send()
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;

        let receipt = pending_tx
            .with_required_confirmations(1)
            .get_receipt()
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;

        Ok(format!("{:#x}", receipt.transaction_hash))
    }

    pub async fn native_balance(&self) -> Result<U256, BlockchainError> {
        self.contract
            .provider()
            .get_balance(self.worker_address)
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))
    }

    pub async fn inspect_address(
        &self,
        target_hex: &str,
    ) -> Result<serde_json::Value, BlockchainError> {
        let target = Address::from_str(target_hex)
            .map_err(|_| BlockchainError::RpcError("invalid address".into()))?;
        let provider = self.contract.provider();
        let native = provider
            .get_balance(target)
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
        let nonce = provider
            .get_transaction_count(target)
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
        let code = provider
            .get_code_at(target)
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
        let token = Address::from_str(DEFAULT_TUSD8_ADDRESS)
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?;
        let balance = IErc20Balance::new(token, provider)
            .balanceOf(target)
            .call()
            .await
            .map_err(|error| BlockchainError::RpcError(error.to_string()))?
            ._0;
        Ok(serde_json::json!({
            "service": "chain_audit",
            "mode": "rpc",
            "address": target_hex,
            "network": "eip155:44005",
            "nativeBalance": native.to_string(),
            "tusd8Balance": balance.to_string(),
            "accountType": if code.is_empty() { "EOA" } else { "Contract" },
            "nonce": nonce,
        }))
    }
}
