// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {EIP712} from "@openzeppelin/contracts/utils/cryptography/EIP712.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

import {INexusEscrow} from "./interfaces/INexusEscrow.sol";
import {EscrowStatus, SessionConfig, TaskEscrow} from "./types/NexusTypes.sol";

/// @title NexusEscrow
/// @notice Escrow and session-key settlement for Nexus402 on QDay Aevum.
/// @dev Principals sign an EIP-712 SessionAuthorization. The ephemeral agent key may then
///      lock tUSD8 up to a per-task cap and a rolling one-hour cap until `expiresAt`.
///      Workers claim with an Ethereum signed message over keccak256(taskId, resultHash).
contract NexusEscrow is INexusEscrow, EIP712, ReentrancyGuard {
    using SafeERC20 for IERC20;

    bytes32 public constant SESSION_TYPEHASH = keccak256(
        "SessionAuthorization(address principal,address agentKey,uint256 hourlyLimit,uint256 perTaskLimit,uint256 expiresAt,uint256 nonce)"
    );

    IERC20 public immutable paymentToken;

    mapping(address => mapping(address => SessionConfig)) private _sessions;
    mapping(address => uint256) public nonces;
    mapping(bytes32 => TaskEscrow) private _tasks;
    mapping(address => mapping(address => uint256)) private _taskNonces;

    constructor(address tusd8_) EIP712("Nexus402 Protocol", "1") {
        require(tusd8_ != address(0), "NexusEscrow: token");
        paymentToken = IERC20(tusd8_);
    }

    /// @inheritdoc INexusEscrow
    function registerSessionWithSig(
        address principal,
        address agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external {
        if (expiresAt <= block.timestamp) revert SessionExpired();
        if (principal == address(0) || agentKey == address(0)) revert InvalidSignature();

        uint256 nonce = nonces[principal];
        bytes32 digest = _hashTypedDataV4(
            keccak256(abi.encode(SESSION_TYPEHASH, principal, agentKey, hourlyLimit, perTaskLimit, expiresAt, nonce))
        );
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, v, r, s);
        if (err != ECDSA.RecoverError.NoError || signer != principal) revert InvalidSignature();

        nonces[principal] = nonce + 1;
        _sessions[principal][agentKey] = SessionConfig({
            agentKey: agentKey,
            hourlyLimit: hourlyLimit,
            perTaskLimit: perTaskLimit,
            expiresAt: expiresAt,
            currentWindow: block.timestamp / 3600,
            spentInWindow: 0,
            isActive: true
        });

        emit SessionAuthorized(principal, agentKey, hourlyLimit, perTaskLimit, expiresAt);
    }

    /// @inheritdoc INexusEscrow
    function lockTaskEscrow(address principal, address worker, uint256 amount, uint256 durationBlocks, bytes32 taskHash)
        external
        nonReentrant
        returns (bytes32 taskId)
    {
        if (amount == 0) revert ZeroAmount();
        if (worker == address(0)) revert InvalidWorkerAddress();

        SessionConfig storage session = _sessions[principal][msg.sender];
        if (!session.isActive || session.agentKey != msg.sender) revert SessionInactive();
        if (block.timestamp >= session.expiresAt) revert SessionExpired();
        if (amount > session.perTaskLimit) revert ExceedsPerTaskLimit();

        uint256 window = block.timestamp / 3600;
        if (window > session.currentWindow) {
            session.currentWindow = window;
            session.spentInWindow = 0;
        }
        uint256 nextSpent = session.spentInWindow + amount;
        if (nextSpent > session.hourlyLimit) revert ExceedsHourlyLimit();
        session.spentInWindow = nextSpent;

        uint256 taskNonce = _taskNonces[principal][msg.sender]++;
        taskId = keccak256(abi.encodePacked(principal, msg.sender, worker, taskNonce, taskHash));
        if (_tasks[taskId].status != EscrowStatus.NONE) revert TaskNotPending();

        uint256 deadline = block.number + durationBlocks;
        _tasks[taskId] = TaskEscrow({
            taskId: taskId,
            principal: principal,
            agentKey: msg.sender,
            worker: worker,
            amount: amount,
            deadline: deadline,
            taskHash: taskHash,
            resultHash: bytes32(0),
            status: EscrowStatus.PENDING
        });

        paymentToken.safeTransferFrom(principal, address(this), amount);
        emit TaskEscrowLocked(taskId, principal, worker, amount, deadline);
    }

    /// @inheritdoc INexusEscrow
    function completeTaskWithProof(bytes32 taskId, bytes32 resultHash, bytes calldata workerSignature)
        external
        nonReentrant
    {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert TaskNotPending();

        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, resultHash)));
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, workerSignature);
        if (err != ECDSA.RecoverError.NoError || signer != task.worker) revert InvalidProofSignature();

        uint256 amount = task.amount;
        address worker = task.worker;
        task.resultHash = resultHash;
        task.status = EscrowStatus.COMPLETED;

        paymentToken.safeTransfer(worker, amount);
        emit TaskCompleted(taskId, worker, resultHash, amount);
    }

    /// @inheritdoc INexusEscrow
    function refundExpiredTask(bytes32 taskId) external nonReentrant {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert TaskNotPending();
        if (block.number <= task.deadline) revert DeadlineNotPassed();

        uint256 amount = task.amount;
        address principal = task.principal;
        task.status = EscrowStatus.REFUNDED;

        paymentToken.safeTransfer(principal, amount);
        emit TaskRefunded(taskId, principal, amount);
    }

    /// @inheritdoc INexusEscrow
    function emergencyRevokeSession(address agentKey) external {
        SessionConfig storage session = _sessions[msg.sender][agentKey];
        if (session.agentKey == address(0) || !session.isActive) revert SessionInactive();
        session.isActive = false;
        emit SessionRevoked(msg.sender, agentKey, "emergency");
    }

    /// @inheritdoc INexusEscrow
    function getSession(address principal, address agentKey) external view returns (SessionConfig memory) {
        return _sessions[principal][agentKey];
    }

    /// @inheritdoc INexusEscrow
    function getTask(bytes32 taskId) external view returns (TaskEscrow memory) {
        return _tasks[taskId];
    }

    function domainSeparator() external view returns (bytes32) {
        return _domainSeparatorV4();
    }
}
