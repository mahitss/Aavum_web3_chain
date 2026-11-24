// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {SessionConfig, TaskEscrow} from "../types/NexusTypes.sol";

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

    /// @notice Release escrow to the worker after an eth-signed proof from the worker key.
    function completeTaskWithProof(bytes32 taskId, bytes32 resultHash, bytes calldata workerSignature) external;

    /// @notice Return the full escrow to the principal once `block.number` is past `deadline`.
    function refundExpiredTask(bytes32 taskId) external;

    /// @notice Principal circuit breaker. Freezes `agentKey` immediately.
    function emergencyRevokeSession(address agentKey) external;

    function getSession(address principal, address agentKey) external view returns (SessionConfig memory);

    function getTask(bytes32 taskId) external view returns (TaskEscrow memory);
}
