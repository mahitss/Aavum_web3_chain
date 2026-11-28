// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {OrgVault, SessionConfig, SessionTab, TaskEscrow, WorkerStats} from "../types/NexusTypes.sol";

/// @title INexusEscrow
/// @notice Agent session delegation and tUSD8 task escrow for the Nexus402 marketplace.
interface INexusEscrow {
    event SessionAuthorized(
        address indexed principal,
        address indexed agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt
    );

    event SessionRevoked(address indexed principal, address indexed agentKey, string reason);

    event TaskEscrowLocked(
        bytes32 indexed taskId, address indexed principal, address indexed worker, uint256 amount, uint256 deadline
    );

    event TaskCompleted(bytes32 indexed taskId, address indexed worker, bytes32 resultHash, uint256 amount);

    event TaskCompletedMetered(
        bytes32 indexed taskId, address indexed worker, bytes32 resultHash, uint256 actualAmount, uint256 refundedAmount
    );

    event TaskRefunded(bytes32 indexed taskId, address indexed principal, uint256 amount);

    error InvalidSignature();
    error SessionExpired();
    error SessionInactive();
    error ExceedsPerTaskLimit();
    error ExceedsHourlyLimit();
    error InvalidWorkerAddress();
    error TaskNotFound();
    error TaskNotPending();
    error DeadlineNotPassed();
    error InvalidProofSignature();
    error ZeroAmount();
    error ExceedsEscrowAmount();
    error WorkerNotAllowlisted();
    error TabClosed();
    error StaleVoucherNonce();
    error VoucherExpired();
    error ExceedsTabDeposit();
    error InvalidVoucherSignature();
    error InvalidEvaluatorSignature();
    error EvaluatorRequired();
    error TaskNotSubmitted();
    error WorkWindowClosed();
    error ChallengeWindowClosed();
    error AuthorizationCaptured();
    error NotAuthorizedToVoid();
    error OrgVaultPaused();
    error ExceedsGlobalHourlyLimit();

    event TaskSubmitted(bytes32 indexed taskId, address indexed worker, bytes32 resultHash);
    event TaskEvaluated(bytes32 indexed taskId, address indexed evaluator, bool approved, bytes32 evaluationHash);
    event TaskRejected(bytes32 indexed taskId, address indexed principal, uint256 amount);
    event AuthorizationVoided(bytes32 indexed taskId, address indexed principal, uint256 amount);

    /// @notice Record a principal's EIP-712 authorization of an ephemeral agent key.
    function registerSessionWithSig(
        address principal,
        address agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external;

    /// @notice Pull `amount` tUSD8 from `principal` and lock it. Caller is the agent key.
    /// @dev `durationBlocks` is added to the current block number to form the refund deadline.
    function lockTaskEscrow(address principal, address worker, uint256 amount, uint256 durationBlocks, bytes32 taskHash)
        external
        returns (bytes32 taskId);

    /// @notice Lock tUSD8 for a job that an evaluator must accept before the worker is paid.
    /// @dev `evaluator == address(0)` keeps the instant direct-settlement path.
    function lockEvaluatedEscrow(
        address principal,
        address worker,
        address evaluator,
        uint256 amount,
        uint256 durationBlocks,
        uint256 challengeWindowBlocks,
        bytes32 taskHash
    ) external returns (bytes32 taskId);

    /// @notice Worker commits a signed result. Funds stay locked until `evaluateTask`.
    function submitWorkForEvaluation(bytes32 taskId, bytes32 resultHash, bytes calldata workerSignature) external;

    /// @notice Evaluator releases the escrow or refunds the principal.
    function evaluateTask(bytes32 taskId, bool approved, bytes32 evaluationHash, bytes calldata evaluatorSignature)
        external;

    /// @notice Return an uncaptured authorization to the principal. Reputation is unchanged.
    function voidAuthorization(bytes32 taskId) external;

    /// @notice Release escrow to the worker after an eth-signed proof from the worker key.
    function completeTaskWithProof(bytes32 taskId, bytes32 resultHash, bytes calldata workerSignature) external;

    /// @notice Pay the worker the measured amount and return unused tUSD8 to the principal.
    function completeTaskMetered(
        bytes32 taskId,
        uint256 actualAmount,
        bytes32 resultHash,
        bytes calldata workerSignature
    ) external;

    /// @notice Return the full escrow to the principal once `block.number` is past `deadline`.
    function refundExpiredTask(bytes32 taskId) external;

    /// @notice Principal circuit breaker. Freezes `agentKey` immediately.
    function emergencyRevokeSession(address agentKey) external;

    function getSession(address principal, address agentKey) external view returns (SessionConfig memory);

    function getTask(bytes32 taskId) external view returns (TaskEscrow memory);

    /// @notice Auto-generated getter for the public worker SLA counters.
    function workerStats(address worker)
        external
        view
        returns (uint256 tasksCompleted, uint256 tasksRefunded, uint256 totalVolumeSettled, uint256 lastActiveBlock);

    /// @notice SLA counters plus reliability in basis points. No terminal tasks reads as 10000.
    function getWorkerReputation(address worker)
        external
        view
        returns (WorkerStats memory stats, uint256 reliabilityBps);

    function setWorkerAllowlist(address worker, bool allowed) external;

    function setEnforceAllowlist(bool enabled) external;

    function openSessionTab(address principal, address worker, uint256 depositAmount, uint256 durationBlocks)
        external
        returns (bytes32 tabId);

    function settleVoucherBatch(
        bytes32 tabId,
        uint256 cumulativeAmount,
        uint256 nonce,
        uint256 validBefore,
        bytes calldata agentSignature
    ) external;

    function getSessionTab(bytes32 tabId) external view returns (SessionTab memory);

    function setOrgVaultLimits(uint256 globalHourlyLimit) external;

    function emergencyPauseAllAgents() external;

    function unpauseAllAgents() external;

    function getOrgVault(address principal) external view returns (OrgVault memory);
}
