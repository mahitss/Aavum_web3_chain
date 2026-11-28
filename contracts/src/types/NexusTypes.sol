// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @notice Lifecycle of a single task escrow. NONE means the id has never been locked.
enum EscrowStatus {
    NONE,
    PENDING,
    COMPLETED,
    REFUNDED,
    SUBMITTED,
    DISPUTED,
    REJECTED
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

/// @notice Immutable SLA counters for one worker address.
struct WorkerStats {
    uint256 tasksCompleted;
    uint256 tasksRefunded;
    uint256 totalVolumeSettled;
    uint256 lastActiveBlock;
}

/// @notice One deposited session tab redeemed by off-chain EIP-712 vouchers.
struct SessionTab {
    bytes32 tabId;
    address principal;
    address agentKey;
    address worker;
    uint256 depositAmount;
    uint256 deadline;
    uint256 lastSettledNonce;
    bool open;
}

/// @notice Off-chain voucher. `cumulativeAmount` is the running total, not the increment.
struct PaymentVoucher {
    bytes32 tabId;
    uint256 cumulativeAmount;
    uint256 nonce;
    uint256 validBefore;
}

/// @notice Shared treasury ceiling for every agent key a principal has delegated.
struct OrgVault {
    uint256 globalHourlyLimit;
    uint256 globalCurrentWindow;
    uint256 globalSpentInWindow;
    bool globalPaused;
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
    address evaluator;
    uint256 challengeWindowBlocks;
}
