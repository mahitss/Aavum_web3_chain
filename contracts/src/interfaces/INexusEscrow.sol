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

    event SessionRevoked(address indexed principal, address indexed agentKey);

    event TaskEscrowLocked(
        bytes32 indexed taskId,
        address indexed principal,
        address indexed agentKey,
        address worker,
        uint256 amount,
        uint256 deadline,
        bytes32 taskHash
    );

    event TaskCompleted(
        bytes32 indexed taskId, address indexed worker, bytes32 resultHash, uint256 amount
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

    /// @notice Record a principal's EIP-712 authorization of an ephemeral agent key.
    function registerSessionWithSig(
        address principal,
        address agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt,
        uint256 nonce,
        bytes calldata signature
    ) external;

    /// @notice Pull `amount` tUSD8 from the session principal and lock it for `taskId`.
    /// @dev Caller must be the authorized agent key. `deadline` is a block number.
    function lockTaskEscrow(
        bytes32 taskId,
        address worker,
        uint256 amount,
        uint256 deadline,
        bytes32 taskHash
    ) external;

    /// @notice Release escrow to the worker after a proof signed by the worker key.
    /// @dev The digest is keccak256(abi.encodePacked(taskId, resultHash)).
    function completeTaskWithProof(bytes32 taskId, bytes32 resultHash, bytes calldata signature)
        external;

    /// @notice Return the full escrow to the principal once `block.number` is past `deadline`.
    function refundExpiredTask(bytes32 taskId) external;

    /// @notice Principal circuit breaker. Freezes `agentKey` immediately.
    function emergencyRevokeSession(address agentKey) external;

    function getSession(address principal, address agentKey)
        external
        view
        returns (SessionConfig memory);

    function getTask(bytes32 taskId) external view returns (TaskEscrow memory);

    function nonces(address principal) external view returns (uint256);

    function agentPrincipal(address agentKey) external view returns (address);

    function settlementToken() external view returns (address);
}
