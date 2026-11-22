// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @notice Lifecycle of a single task escrow. NONE means the id has never been locked.
enum EscrowStatus {
    NONE,
    PENDING,
    COMPLETED,
    REFUNDED
}

/// @notice Scoped spending authority delegated from a principal to an ephemeral agent key.
struct SessionConfig {
    address agentKey;
    uint256 hourlyLimit;
    uint256 perTaskLimit;
    uint256 expiresAt;
    uint256 currentWindow;
    uint256 spentInWindow;
    bool isActive;
}

/// @notice Funds locked for one unit of agent work, settled in tUSD8 base units (1e6).
struct TaskEscrow {
    bytes32 taskId;
    address principal;
    address agentKey;
    address worker;
    uint256 amount;
    uint256 deadline;
    bytes32 taskHash;
    bytes32 resultHash;
    EscrowStatus status;
}
