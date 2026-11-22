use alloy::network::{Ethereum, EthereumWallet};
use alloy::primitives::{Address, Bytes, B256, U256};
use alloy::providers::fillers::{
    ChainIdFiller, FillProvider, GasFiller, JoinFill, NonceFiller, WalletFiller,
};
use alloy::providers::{Identity, ProviderBuilder, RootProvider};
use alloy::signers::local::PrivateKeySigner;
use alloy::signers::Signer;
use alloy::sol;
use crate::protocol::{self, CHAIN_ID, PAYMENT_AMOUNT_UNITS, RPC_URL};

sol! {
    #[sol(rpc)]
    interface INexusEscrow {
        function getTask(bytes32 taskId) external view returns (
            bytes32 taskId,
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
            bytes calldata workerSignature
        ) external;
    }
}

type HttpTransport = alloy::transports::http::ReqwestTransport;
type AevumFiller = JoinFill<
    JoinFill<JoinFill<JoinFill<Identity, GasFiller>, NonceFiller>, ChainIdFiller>,
    WalletFiller<EthereumWallet>,
>;
type AevumProvider = FillProvider<AevumFiller, RootProvider<HttpTransport>, HttpTransport, Ethereum>;

#[derive(Debug)]
pub enum WorkerError {
    Config(String),
    Rpc(String),
    Sign(String),
}

impl std::fmt::Display for WorkerError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Config(message) | Self::Rpc(message) | Self::Sign(message) => {
                formatter.write_str(message)
            }
        }
    }
}

impl std::error::Error for WorkerError {}

#[derive(Clone)]
pub struct EscrowEngine {
    contract: INexusEscrow::INexusEscrowInstance<HttpTransport, AevumProvider>,
    signer: PrivateKeySigner,
    worker: Address,
}

impl EscrowEngine {
    pub fn connect(private_key: &str, escrow: &str) -> Result<Self, WorkerError> {
        let mut signer = private_key
            .parse::<PrivateKeySigner>()
            .map_err(|error| WorkerError::Config(format!("WORKER_PRIVATE_KEY: {error}")))?;
        signer.set_chain_id(Some(CHAIN_ID));
        let worker = signer.address();
        let escrow: Address = escrow
            .parse()
            .map_err(|error| WorkerError::Config(format!("NEXUS_ESCROW_ADDRESS: {error}")))?;
        let wallet = EthereumWallet::from(signer.clone());
        let rpc = RPC_URL
            .parse()
            .map_err(|error| WorkerError::Config(format!("rpc: {error}")))?;
        let provider = ProviderBuilder::new()
            .with_recommended_fillers()
            .wallet(wallet)
            .on_http(rpc);
        Ok(Self {
            contract: INexusEscrow::new(escrow, provider),
            signer,
            worker,
        })
    }

    pub fn worker_address(&self) -> Address {
        self.worker
    }

    pub async fn verify_escrow(
        &self,
        task_id: [u8; 32],
        expected_task_hash: [u8; 32],
    ) -> Result<bool, WorkerError> {
        let task = self
            .contract
            .getTask(B256::from(task_id))
            .call()
            .await
            .map_err(|error| WorkerError::Rpc(error.to_string()))?;

        let pending = task.status == 1;
        let payable_to_worker = task.worker == self.worker;
        let funded = task.amount >= U256::from(PAYMENT_AMOUNT_UNITS);
        let commitment_matches = task.taskHash == B256::from(expected_task_hash);
        Ok(pending && payable_to_worker && funded && commitment_matches)
    }

    pub async fn claim_payout(
        &self,
        task_id: [u8; 32],
        result_hash: [u8; 32],
    ) -> Result<String, WorkerError> {
        let digest = B256::from(protocol::eth_signed_proof_hash(task_id, result_hash));
        let signature = self
            .signer
            .sign_hash(&digest)
            .await
            .map_err(|error| WorkerError::Sign(error.to_string()))?;
        let mut signature_bytes = signature.as_bytes();
        if signature_bytes[64] < 27 {
            signature_bytes[64] += 27;
        }

        let call = self.contract.completeTaskWithProof(
            B256::from(task_id),
            B256::from(result_hash),
            Bytes::from(signature_bytes.to_vec()),
        );
        let pending = call
            .send()
            .await
            .map_err(|error| WorkerError::Rpc(error.to_string()))?;
        let receipt = pending
            .with_required_confirmations(1)
            .get_receipt()
            .await
            .map_err(|error| WorkerError::Rpc(error.to_string()))?;
        Ok(format!("{:#x}", receipt.transaction_hash))
    }
}
