// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {EIP712} from "@openzeppelin/contracts/utils/cryptography/EIP712.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

import {INexusEscrow} from "./interfaces/INexusEscrow.sol";
import {EscrowStatus, SessionConfig, TaskEscrow} from "./types/NexusTypes.sol";

/// @title NexusEscrow
/// @notice Escrow and session-key settlement for Nexus402 on QDay Aevum.
/// @dev Principals sign an EIP-712 SessionAuthorization. The ephemeral agent key
///      may then lock tUSD8 up to a per-task cap and a rolling one-hour cap
///      until `expiresAt`. Workers claim with an ECDSA proof over the task result.
contract NexusEscrow is INexusEscrow, EIP712, ReentrancyGuard {
    using SafeERC20 for IERC20;

    bytes32 public constant SESSION_TYPEHASH = keccak256(
        "SessionAuthorization(address principal,address agentKey,uint256 hourlyLimit,uint256 perTaskLimit,uint256 expiresAt,uint256 nonce)"
    );

    address public immutable settlementToken;

    mapping(address principal => mapping(address agentKey => SessionConfig)) private _sessions;
    mapping(address agentKey => address principal) private _agentPrincipal;
    mapping(bytes32 taskId => TaskEscrow) private _tasks;
    mapping(address principal => uint256 nonce) private _nonces;

    constructor(address tusd8_) EIP712("Nexus402 Protocol", "1") {
        require(tusd8_ != address(0), "NexusEscrow: token");
        settlementToken = tusd8_;
    }

    /// @inheritdoc INexusEscrow
    function registerSessionWithSig(
        address principal,
        address agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt,
        uint256 nonce,
        bytes calldata signature
    ) external {
        if (principal == address(0) || agentKey == address(0)) revert InvalidSignature();
        if (nonce != _nonces[principal]) revert InvalidSignature();

        bytes32 digest = _hashTypedDataV4(
            keccak256(
                abi.encode(
                    SESSION_TYPEHASH, principal, agentKey, hourlyLimit, perTaskLimit, expiresAt, nonce
                )
            )
        );
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, signature);
        if (err != ECDSA.RecoverError.NoError || signer != principal) revert InvalidSignature();
        if (expiresAt <= block.timestamp) revert SessionExpired();

        address bound = _agentPrincipal[agentKey];
        if (bound != address(0) && bound != principal) revert SessionInactive();

        _nonces[principal] = nonce + 1;
        _agentPrincipal[agentKey] = principal;
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
    function lockTaskEscrow(
        bytes32 taskId,
        address worker,
        uint256 amount,
        uint256 deadline,
        bytes32 taskHash
    ) external nonReentrant {
        if (amount == 0) revert ZeroAmount();
        if (worker == address(0)) revert InvalidWorkerAddress();

        address principal = _agentPrincipal[msg.sender];
        if (principal == address(0)) revert SessionInactive();

        SessionConfig storage session = _sessions[principal][msg.sender];
        if (!session.isActive) revert SessionInactive();
        if (block.timestamp >= session.expiresAt) revert SessionExpired();
        if (amount > session.perTaskLimit) revert ExceedsPerTaskLimit();
        if (_tasks[taskId].status != EscrowStatus.NONE) revert TaskNotPending();

        uint256 window = block.timestamp / 3600;
        if (session.currentWindow != window) {
            session.currentWindow = window;
            session.spentInWindow = 0;
        }
        uint256 nextSpent = session.spentInWindow + amount;
        if (nextSpent > session.hourlyLimit) revert ExceedsHourlyLimit();

        session.spentInWindow = nextSpent;
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

        IERC20(settlementToken).safeTransferFrom(principal, address(this), amount);

        emit TaskEscrowLocked(taskId, principal, msg.sender, worker, amount, deadline, taskHash);
    }

    /// @inheritdoc INexusEscrow
    function completeTaskWithProof(bytes32 taskId, bytes32 resultHash, bytes calldata signature)
        external
        nonReentrant
    {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert TaskNotPending();

        bytes32 digest = keccak256(abi.encodePacked(taskId, resultHash));
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, signature);
        if (err != ECDSA.RecoverError.NoError || signer != task.worker) {
            revert InvalidProofSignature();
        }

        uint256 amount = task.amount;
        address worker = task.worker;
        task.resultHash = resultHash;
        task.status = EscrowStatus.COMPLETED;

        IERC20(settlementToken).safeTransfer(worker, amount);
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

        IERC20(settlementToken).safeTransfer(principal, amount);
        emit TaskRefunded(taskId, principal, amount);
    }

    /// @inheritdoc INexusEscrow
    function emergencyRevokeSession(address agentKey) external {
        SessionConfig storage session = _sessions[msg.sender][agentKey];
        if (session.agentKey == address(0) || !session.isActive) revert SessionInactive();
        session.isActive = false;
        emit SessionRevoked(msg.sender, agentKey);
    }

    /// @inheritdoc INexusEscrow
    function getSession(address principal, address agentKey)
        external
        view
        returns (SessionConfig memory)
    {
        return _sessions[principal][agentKey];
    }

    /// @inheritdoc INexusEscrow
    function getTask(bytes32 taskId) external view returns (TaskEscrow memory) {
        return _tasks[taskId];
    }

    /// @inheritdoc INexusEscrow
    function nonces(address principal) external view returns (uint256) {
        return _nonces[principal];
    }

    /// @inheritdoc INexusEscrow
    function agentPrincipal(address agentKey) external view returns (address) {
        return _agentPrincipal[agentKey];
    }

    function domainSeparator() external view returns (bytes32) {
        return _domainSeparatorV4();
    }
}
