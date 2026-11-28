// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {EIP712} from "@openzeppelin/contracts/utils/cryptography/EIP712.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

import {INexusEscrow} from "./interfaces/INexusEscrow.sol";
import {EscrowStatus, OrgVault, SessionConfig, SessionTab, TaskEscrow, WorkerStats} from "./types/NexusTypes.sol";

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
    bytes32 public constant PAYMENT_VOUCHER_TYPEHASH = keccak256(
        "PaymentVoucher(bytes32 tabId,uint256 cumulativeAmount,uint256 nonce,uint256 validBefore)"
    );

    IERC20 public immutable paymentToken;

    mapping(address => mapping(address => SessionConfig)) private _sessions;
    mapping(address => uint256) public nonces;
    mapping(bytes32 => TaskEscrow) private _tasks;
    mapping(address => mapping(address => uint256)) private _taskNonces;
    mapping(address => WorkerStats) public workerStats;
    mapping(address => mapping(address => bool)) public allowedWorkers;
    mapping(address => bool) public enforceAllowlist;
    mapping(bytes32 => SessionTab) private _tabs;
    mapping(address => OrgVault) private _vaults;
    mapping(address => mapping(address => uint256)) private _tabNonces;

    event SessionTabOpened(
        bytes32 indexed tabId, address indexed principal, address indexed worker, uint256 depositAmount, uint256 deadline
    );
    event VoucherBatchSettled(
        bytes32 indexed tabId, address indexed worker, uint256 cumulativeAmount, uint256 refundedAmount, uint256 nonce
    );

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
        return _openEscrow(principal, worker, address(0), amount, durationBlocks, 0, taskHash);
    }

    /// @inheritdoc INexusEscrow
    function lockEvaluatedEscrow(
        address principal,
        address worker,
        address evaluator,
        uint256 amount,
        uint256 durationBlocks,
        uint256 challengeWindowBlocks,
        bytes32 taskHash
    ) external nonReentrant returns (bytes32 taskId) {
        return _openEscrow(principal, worker, evaluator, amount, durationBlocks, challengeWindowBlocks, taskHash);
    }

    /// @inheritdoc INexusEscrow
    function completeTaskWithProof(bytes32 taskId, bytes32 resultHash, bytes calldata workerSignature)
        external
        nonReentrant
    {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert TaskNotPending();
        if (task.evaluator != address(0)) revert EvaluatorRequired();

        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, resultHash)));
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, workerSignature);
        if (err != ECDSA.RecoverError.NoError || signer != task.worker) revert InvalidProofSignature();

        uint256 amount = task.amount;
        address worker = task.worker;
        task.resultHash = resultHash;
        task.status = EscrowStatus.COMPLETED;
        _recordCompletion(worker, amount);

        paymentToken.safeTransfer(worker, amount);
        emit TaskCompleted(taskId, worker, resultHash, amount);
    }

    /// @inheritdoc INexusEscrow
    function completeTaskMetered(
        bytes32 taskId,
        uint256 actualAmount,
        bytes32 resultHash,
        bytes calldata workerSignature
    ) external nonReentrant {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert TaskNotPending();
        if (task.evaluator != address(0)) revert EvaluatorRequired();
        if (actualAmount == 0 || actualAmount > task.amount) revert ExceedsEscrowAmount();

        bytes32 digest =
            MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, actualAmount, resultHash)));
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, workerSignature);
        if (err != ECDSA.RecoverError.NoError || signer != task.worker) revert InvalidProofSignature();

        uint256 unusedRefund = task.amount - actualAmount;
        address worker = task.worker;
        address principal = task.principal;
        address agentKey = task.agentKey;
        task.resultHash = resultHash;
        task.status = EscrowStatus.COMPLETED;
        _recordCompletion(worker, actualAmount);

        paymentToken.safeTransfer(worker, actualAmount);
        if (unusedRefund > 0) {
            paymentToken.safeTransfer(principal, unusedRefund);
            _creditWindow(principal, agentKey, unusedRefund);
        }
        emit TaskCompletedMetered(taskId, worker, resultHash, actualAmount, unusedRefund);
    }

    /// @inheritdoc INexusEscrow
    function refundExpiredTask(bytes32 taskId) external nonReentrant {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert TaskNotPending();
        if (block.number <= task.deadline) revert DeadlineNotPassed();

        uint256 amount = task.amount;
        address principal = task.principal;
        address worker = task.worker;
        task.status = EscrowStatus.REFUNDED;
        _recordRefund(worker);

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

    /// @inheritdoc INexusEscrow
    function getWorkerReputation(address worker)
        external
        view
        returns (WorkerStats memory stats, uint256 reliabilityBps)
    {
        stats = workerStats[worker];
        uint256 terminal = stats.tasksCompleted + stats.tasksRefunded;
        reliabilityBps = terminal == 0 ? 10_000 : (stats.tasksCompleted * 10_000) / terminal;
    }

    function _recordCompletion(address worker, uint256 volume) internal {
        WorkerStats storage stats = workerStats[worker];
        stats.tasksCompleted += 1;
        stats.totalVolumeSettled += volume;
        stats.lastActiveBlock = block.number;
    }

    function _recordRefund(address worker) internal {
        WorkerStats storage stats = workerStats[worker];
        stats.tasksRefunded += 1;
        stats.lastActiveBlock = block.number;
    }

    function domainSeparator() external view returns (bytes32) {
        return _domainSeparatorV4();
    }

    /// @notice Principal chooses whether session keys may pay only approved workers.
    function setEnforceAllowlist(bool enabled) external {
        enforceAllowlist[msg.sender] = enabled;
    }

    /// @notice Principal approves or removes one worker address.
    function setWorkerAllowlist(address worker, bool allowed) external {
        if (worker == address(0)) revert InvalidWorkerAddress();
        allowedWorkers[msg.sender][worker] = allowed;
    }

    /// @notice Deposit one tUSD8 tab. Later calls are off-chain vouchers against this balance.
    function openSessionTab(address principal, address worker, uint256 depositAmount, uint256 durationBlocks)
        external
        nonReentrant
        returns (bytes32 tabId)
    {
        if (depositAmount == 0) revert ZeroAmount();
        if (worker == address(0)) revert InvalidWorkerAddress();
        _requireAllowlisted(principal, worker);
        _chargeOrg(principal, depositAmount);

        SessionConfig storage session = _sessions[principal][msg.sender];
        if (!session.isActive || session.agentKey != msg.sender) revert SessionInactive();
        if (block.timestamp >= session.expiresAt) revert SessionExpired();

        uint256 window = block.timestamp / 3600;
        if (window > session.currentWindow) {
            session.currentWindow = window;
            session.spentInWindow = 0;
        }
        uint256 nextSpent = session.spentInWindow + depositAmount;
        if (nextSpent > session.hourlyLimit) revert ExceedsHourlyLimit();
        session.spentInWindow = nextSpent;

        uint256 tabNonce = _tabNonces[principal][msg.sender]++;
        tabId = keccak256(abi.encodePacked(principal, msg.sender, worker, tabNonce, depositAmount, block.number));
        uint256 deadline = block.number + durationBlocks;
        _tabs[tabId] = SessionTab({
            tabId: tabId,
            principal: principal,
            agentKey: msg.sender,
            worker: worker,
            depositAmount: depositAmount,
            deadline: deadline,
            lastSettledNonce: 0,
            open: true
        });
        paymentToken.safeTransferFrom(principal, address(this), depositAmount);
        emit SessionTabOpened(tabId, principal, worker, depositAmount, deadline);
    }

    /// @notice Redeem the highest voucher. One transaction pays the worker and returns the unused deposit.
    function settleVoucherBatch(
        bytes32 tabId,
        uint256 cumulativeAmount,
        uint256 nonce,
        uint256 validBefore,
        bytes calldata agentSignature
    ) external nonReentrant {
        SessionTab storage tab = _tabs[tabId];
        if (!tab.open) revert TabClosed();
        if (block.timestamp > validBefore) revert VoucherExpired();
        if (cumulativeAmount > tab.depositAmount) revert ExceedsTabDeposit();
        if (nonce <= tab.lastSettledNonce) revert StaleVoucherNonce();
        _verifyVoucher(tab.agentKey, tabId, cumulativeAmount, nonce, validBefore, agentSignature);

        uint256 refunded = tab.depositAmount - cumulativeAmount;
        address worker = tab.worker;
        address principal = tab.principal;
        address agentKey = tab.agentKey;
        tab.lastSettledNonce = nonce;
        tab.open = false;
        _payClosedTab(tabId, worker, principal, agentKey, cumulativeAmount, refunded, nonce);
    }

    function _verifyVoucher(
        address agentKey,
        bytes32 tabId,
        uint256 cumulativeAmount,
        uint256 nonce,
        uint256 validBefore,
        bytes calldata agentSignature
    ) internal view {
        bytes32 digest = _hashTypedDataV4(
            keccak256(abi.encode(PAYMENT_VOUCHER_TYPEHASH, tabId, cumulativeAmount, nonce, validBefore))
        );
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, agentSignature);
        if (err != ECDSA.RecoverError.NoError || signer != agentKey) revert InvalidVoucherSignature();
    }

    function _payClosedTab(
        bytes32 tabId,
        address worker,
        address principal,
        address agentKey,
        uint256 cumulativeAmount,
        uint256 refunded,
        uint256 nonce
    ) internal {
        if (cumulativeAmount > 0) {
            paymentToken.safeTransfer(worker, cumulativeAmount);
            _recordCompletion(worker, cumulativeAmount);
        }
        if (refunded > 0) {
            paymentToken.safeTransfer(principal, refunded);
            _creditWindow(principal, agentKey, refunded);
        }
        emit VoucherBatchSettled(tabId, worker, cumulativeAmount, refunded, nonce);
    }

    function getSessionTab(bytes32 tabId) external view returns (SessionTab memory) {
        return _tabs[tabId];
    }

    function _requireAllowlisted(address principal, address worker) internal view {
        if (enforceAllowlist[principal] && !allowedWorkers[principal][worker]) revert WorkerNotAllowlisted();
    }

    function _openEscrow(
        address principal,
        address worker,
        address evaluator,
        uint256 amount,
        uint256 durationBlocks,
        uint256 challengeWindowBlocks,
        bytes32 taskHash
    ) internal returns (bytes32 taskId) {
        if (amount == 0) revert ZeroAmount();
        if (worker == address(0)) revert InvalidWorkerAddress();
        _requireAllowlisted(principal, worker);
        _chargeOrg(principal, amount);
        _chargeSession(principal, amount);

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
            status: EscrowStatus.PENDING,
            evaluator: evaluator,
            challengeWindowBlocks: challengeWindowBlocks
        });
        paymentToken.safeTransferFrom(principal, address(this), amount);
        emit TaskEscrowLocked(taskId, principal, worker, amount, deadline);
    }

    function _chargeSession(address principal, uint256 amount) internal {
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
    }

    /// @inheritdoc INexusEscrow
    function submitWorkForEvaluation(bytes32 taskId, bytes32 resultHash, bytes calldata workerSignature)
        external
        nonReentrant
    {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert TaskNotPending();
        if (block.number > task.deadline) revert WorkWindowClosed();

        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, resultHash)));
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, workerSignature);
        if (err != ECDSA.RecoverError.NoError || signer != task.worker) revert InvalidProofSignature();

        address worker = task.worker;
        task.resultHash = resultHash;
        task.status = EscrowStatus.SUBMITTED;
        emit TaskSubmitted(taskId, worker, resultHash);
    }

    /// @inheritdoc INexusEscrow
    function evaluateTask(bytes32 taskId, bool approved, bytes32 evaluationHash, bytes calldata evaluatorSignature)
        external
        nonReentrant
    {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.SUBMITTED) revert TaskNotSubmitted();
        if (block.number > task.deadline + task.challengeWindowBlocks) revert ChallengeWindowClosed();
        _verifyEvaluator(task.evaluator, taskId, approved, evaluationHash, evaluatorSignature);
        if (approved) {
            _acceptEvaluation(taskId, evaluationHash);
        } else {
            _rejectEvaluation(taskId, evaluationHash);
        }
    }

    /// @inheritdoc INexusEscrow
    function voidAuthorization(bytes32 taskId) external nonReentrant {
        TaskEscrow storage task = _tasks[taskId];
        if (task.status == EscrowStatus.NONE) revert TaskNotFound();
        if (task.status != EscrowStatus.PENDING) revert AuthorizationCaptured();
        if (msg.sender != task.worker && block.number <= task.deadline) revert NotAuthorizedToVoid();

        uint256 amount = task.amount;
        address principal = task.principal;
        address agentKey = task.agentKey;
        task.status = EscrowStatus.REFUNDED;
        paymentToken.safeTransfer(principal, amount);
        _creditWindow(principal, agentKey, amount);
        emit AuthorizationVoided(taskId, principal, amount);
    }

    function _verifyEvaluator(
        address evaluator,
        bytes32 taskId,
        bool approved,
        bytes32 evaluationHash,
        bytes calldata evaluatorSignature
    ) internal pure {
        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(
            keccak256(abi.encodePacked(taskId, approved, evaluationHash))
        );
        (address signer, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, evaluatorSignature);
        if (err != ECDSA.RecoverError.NoError || signer != evaluator) revert InvalidEvaluatorSignature();
    }

    function _acceptEvaluation(bytes32 taskId, bytes32 evaluationHash) internal {
        TaskEscrow storage task = _tasks[taskId];
        uint256 amount = task.amount;
        address worker = task.worker;
        address evaluator = task.evaluator;
        bytes32 resultHash = task.resultHash;
        task.status = EscrowStatus.COMPLETED;
        _recordCompletion(worker, amount);
        paymentToken.safeTransfer(worker, amount);
        emit TaskEvaluated(taskId, evaluator, true, evaluationHash);
        emit TaskCompleted(taskId, worker, resultHash, amount);
    }

    function _rejectEvaluation(bytes32 taskId, bytes32 evaluationHash) internal {
        TaskEscrow storage task = _tasks[taskId];
        uint256 amount = task.amount;
        address principal = task.principal;
        address worker = task.worker;
        address agentKey = task.agentKey;
        address evaluator = task.evaluator;
        task.status = EscrowStatus.REJECTED;
        _recordRefund(worker);
        paymentToken.safeTransfer(principal, amount);
        _creditWindow(principal, agentKey, amount);
        emit TaskEvaluated(taskId, evaluator, false, evaluationHash);
        emit TaskRejected(taskId, principal, amount);
    }

    function _creditWindow(address principal, address agentKey, uint256 amount) internal {
        if (amount == 0) return;
        SessionConfig storage session = _sessions[principal][agentKey];
        uint256 window = block.timestamp / 3600;
        if (window == session.currentWindow && session.spentInWindow >= amount) {
            session.spentInWindow -= amount;
        }
        _creditOrg(principal, amount);
    }

    /// @notice Principal sets the shared hourly ceiling. Zero disables the global cap.
    function setOrgVaultLimits(uint256 globalHourlyLimit) external {
        _vaults[msg.sender].globalHourlyLimit = globalHourlyLimit;
    }

    /// @notice Freeze every agent key that spends from the caller's treasury.
    function emergencyPauseAllAgents() external {
        _vaults[msg.sender].globalPaused = true;
    }

    /// @notice Lift the treasury-wide pause. Individual session revocations stay revoked.
    function unpauseAllAgents() external {
        _vaults[msg.sender].globalPaused = false;
    }

    function getOrgVault(address principal) external view returns (OrgVault memory) {
        return _vaults[principal];
    }

    function _chargeOrg(address principal, uint256 amount) internal {
        OrgVault storage vault = _vaults[principal];
        if (vault.globalPaused) revert OrgVaultPaused();
        if (vault.globalHourlyLimit == 0) return;
        uint256 window = block.timestamp / 3600;
        if (window > vault.globalCurrentWindow) {
            vault.globalCurrentWindow = window;
            vault.globalSpentInWindow = 0;
        }
        uint256 nextSpent = vault.globalSpentInWindow + amount;
        if (nextSpent > vault.globalHourlyLimit) revert ExceedsGlobalHourlyLimit();
        vault.globalSpentInWindow = nextSpent;
    }

    function _creditOrg(address principal, uint256 amount) internal {
        if (amount == 0) return;
        OrgVault storage vault = _vaults[principal];
        if (vault.globalHourlyLimit == 0) return;
        uint256 window = block.timestamp / 3600;
        if (window == vault.globalCurrentWindow && vault.globalSpentInWindow >= amount) {
            vault.globalSpentInWindow -= amount;
        }
    }
}
