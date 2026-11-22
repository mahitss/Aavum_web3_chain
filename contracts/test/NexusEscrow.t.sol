// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";
import {EscrowStatus, SessionConfig, TaskEscrow} from "../src/types/NexusTypes.sol";

contract MockTUSD8 is ERC20 {
    constructor() ERC20("Test USD8", "tUSD8") {}

    function decimals() public pure override returns (uint8) {
        return 6;
    }

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }
}

/// @dev Token that calls back into the escrow during transferFrom.
contract ReentrantTUSD8 is ERC20 {
    NexusEscrow public escrow;
    address public worker;
    bytes32 public outerTask;
    uint256 public amount;
    bool internal entered;

    constructor() ERC20("Reentrant USD8", "rUSD8") {}

    function decimals() public pure override returns (uint8) {
        return 6;
    }

    function mint(address to, uint256 value) external {
        _mint(to, value);
    }

    function arm(NexusEscrow escrow_, address worker_, bytes32 outerTask_, uint256 amount_) external {
        escrow = escrow_;
        worker = worker_;
        outerTask = outerTask_;
        amount = amount_;
    }

    function trigger() external {
        escrow.lockTaskEscrow(outerTask, worker, amount, block.number + 50, keccak256("payload"));
    }

    function transferFrom(address from, address to, uint256 value) public override returns (bool) {
        if (!entered) {
            entered = true;
            escrow.lockTaskEscrow(keccak256("reenter"), worker, amount, block.number + 50, keccak256("re"));
        }
        return super.transferFrom(from, to, value);
    }
}

contract NexusEscrowTest is Test {
    event SessionRevoked(address indexed principal, address indexed agentKey);

    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant WORKER_PK = 0xB0B;
    uint256 internal constant HOURLY = 1_000_000;
    uint256 internal constant PER_TASK = 250_000;
    uint256 internal constant FEE = 250_000;

    address internal principal;
    address internal agent;
    address internal worker;
    MockTUSD8 internal token;
    NexusEscrow internal escrow;

    function setUp() public {
        principal = vm.addr(PRINCIPAL_PK);
        agent = vm.addr(AGENT_PK);
        worker = vm.addr(WORKER_PK);

        token = new MockTUSD8();
        escrow = new NexusEscrow(address(token));
        token.mint(principal, 100_000_000);

        vm.prank(principal);
        token.approve(address(escrow), type(uint256).max);
        _authorize(agent, HOURLY, PER_TASK, block.timestamp + 1 days);
    }

    function testSessionDelegationStoresScopedAuthority() public view {
        SessionConfig memory session = escrow.getSession(principal, agent);
        assertEq(session.agentKey, agent);
        assertEq(session.hourlyLimit, HOURLY);
        assertEq(session.perTaskLimit, PER_TASK);
        assertEq(session.spentInWindow, 0);
        assertTrue(session.isActive);
        assertEq(escrow.agentPrincipal(agent), principal);
        assertEq(escrow.nonces(principal), 1);
        assertEq(escrow.settlementToken(), address(token));

        (bytes1 fields, string memory name, string memory version,, address verifying,,) =
            escrow.eip712Domain();
        assertEq(fields, hex"0f");
        assertEq(name, "Nexus402 Protocol");
        assertEq(version, "1");
        assertEq(verifying, address(escrow));
    }

    function testRejectsInvalidSessionSignature() public {
        uint256 nonce = escrow.nonces(principal);
        address freshAgent = vm.addr(0xF4E5);
        uint256 expiresAt = block.timestamp + 1 days;
        bytes memory sig =
            _signSession(escrow, AGENT_PK, principal, freshAgent, HOURLY, PER_TASK, expiresAt, nonce);

        vm.expectRevert(INexusEscrow.InvalidSignature.selector);
        escrow.registerSessionWithSig(principal, freshAgent, HOURLY, PER_TASK, expiresAt, nonce, sig);
        assertFalse(escrow.getSession(principal, freshAgent).isActive);
        assertEq(escrow.nonces(principal), nonce);
    }

    function testRejectsReplayedSessionNonce() public {
        uint256 expiresAt = block.timestamp + 1 days;
        bytes memory sig = _signSession(escrow, PRINCIPAL_PK, principal, agent, HOURLY, PER_TASK, expiresAt, 0);
        vm.expectRevert(INexusEscrow.InvalidSignature.selector);
        escrow.registerSessionWithSig(principal, agent, HOURLY, PER_TASK, expiresAt, 0, sig);
    }

    function testRejectsCrossChainSessionReplay() public {
        uint256 nonce = escrow.nonces(principal);
        address freshAgent = vm.addr(0xC4A11);
        uint256 expiresAt = block.timestamp + 1 days;
        bytes memory sig =
            _signSession(escrow, PRINCIPAL_PK, principal, freshAgent, HOURLY, PER_TASK, expiresAt, nonce);

        vm.chainId(1);
        vm.expectRevert(INexusEscrow.InvalidSignature.selector);
        escrow.registerSessionWithSig(principal, freshAgent, HOURLY, PER_TASK, expiresAt, nonce, sig);
    }

    function testRejectsExpiredAuthorization() public {
        uint256 nonce = escrow.nonces(principal);
        address freshAgent = vm.addr(0xE991);
        uint256 expiresAt = block.timestamp;
        bytes memory sig =
            _signSession(escrow, PRINCIPAL_PK, principal, freshAgent, HOURLY, PER_TASK, expiresAt, nonce);
        vm.expectRevert(INexusEscrow.SessionExpired.selector);
        escrow.registerSessionWithSig(principal, freshAgent, HOURLY, PER_TASK, expiresAt, nonce, sig);
    }

    function testHappyPathSettlement() public {
        bytes32 taskId = keccak256("task-happy");
        bytes32 taskHash = keccak256("payload-happy");
        uint256 deadline = block.number + 50;

        vm.prank(agent);
        escrow.lockTaskEscrow(taskId, worker, FEE, deadline, taskHash);

        assertEq(token.balanceOf(address(escrow)), FEE);
        assertEq(token.balanceOf(principal), 100_000_000 - FEE);

        TaskEscrow memory locked = escrow.getTask(taskId);
        assertEq(uint8(locked.status), uint8(EscrowStatus.PENDING));
        assertEq(locked.principal, principal);
        assertEq(locked.worker, worker);
        assertEq(locked.deadline, deadline);
        assertEq(locked.taskHash, taskHash);

        bytes32 resultHash = keccak256("result-happy");
        bytes memory proof = _signProof(WORKER_PK, taskId, resultHash);
        escrow.completeTaskWithProof(taskId, resultHash, proof);

        TaskEscrow memory done = escrow.getTask(taskId);
        assertEq(uint8(done.status), uint8(EscrowStatus.COMPLETED));
        assertEq(done.resultHash, resultHash);
        assertEq(token.balanceOf(worker), FEE);
        assertEq(token.balanceOf(address(escrow)), 0);
    }

    function testRejectsInvalidProof() public {
        bytes32 taskId = keccak256("task-bad-proof");
        vm.prank(agent);
        escrow.lockTaskEscrow(taskId, worker, FEE, block.number + 50, keccak256("payload"));

        bytes32 resultHash = keccak256("result");
        bytes memory proof = _signProof(AGENT_PK, taskId, resultHash);
        vm.expectRevert(INexusEscrow.InvalidProofSignature.selector);
        escrow.completeTaskWithProof(taskId, resultHash, proof);

        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.PENDING));
        assertEq(token.balanceOf(worker), 0);
        assertEq(token.balanceOf(address(escrow)), FEE);
    }

    function testHourlyVelocityLimitResetsOnNewWindow() public {
        bytes32 first = keccak256("h1");
        vm.prank(agent);
        escrow.lockTaskEscrow(first, worker, FEE, block.number + 50, keccak256("p1"));
        vm.prank(agent);
        escrow.lockTaskEscrow(keccak256("h2"), worker, FEE, block.number + 50, keccak256("p2"));
        vm.prank(agent);
        escrow.lockTaskEscrow(keccak256("h3"), worker, FEE, block.number + 50, keccak256("p3"));
        vm.prank(agent);
        escrow.lockTaskEscrow(keccak256("h4"), worker, FEE, block.number + 50, keccak256("p4"));

        assertEq(escrow.getSession(principal, agent).spentInWindow, HOURLY);

        uint256 window = block.timestamp / 3600;
        uint256 lastSecond = (window + 1) * 3600 - 1;

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.ExceedsHourlyLimit.selector);
        escrow.lockTaskEscrow(keccak256("h5"), worker, FEE, block.number + 50, keccak256("p5"));

        vm.warp(lastSecond);
        vm.prank(agent);
        vm.expectRevert(INexusEscrow.ExceedsHourlyLimit.selector);
        escrow.lockTaskEscrow(keccak256("h5"), worker, FEE, block.number + 50, keccak256("p5"));

        vm.warp(lastSecond + 1);
        vm.prank(agent);
        escrow.lockTaskEscrow(keccak256("h5"), worker, FEE, block.number + 50, keccak256("p5"));

        SessionConfig memory session = escrow.getSession(principal, agent);
        assertEq(session.currentWindow, window + 1);
        assertEq(session.spentInWindow, FEE);
        assertEq(uint8(escrow.getTask(first).status), uint8(EscrowStatus.PENDING));
    }

    function testPerTaskLimitAndZeroAmountAndWorker() public {
        vm.prank(agent);
        vm.expectRevert(INexusEscrow.ExceedsPerTaskLimit.selector);
        escrow.lockTaskEscrow(keccak256("over"), worker, PER_TASK + 1, block.number + 10, bytes32(0));

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.ZeroAmount.selector);
        escrow.lockTaskEscrow(keccak256("zero"), worker, 0, block.number + 10, bytes32(0));

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.InvalidWorkerAddress.selector);
        escrow.lockTaskEscrow(keccak256("noworker"), address(0), FEE, block.number + 10, bytes32(0));
    }

    function testPerTaskLimitAllowsExactCap() public {
        vm.prank(agent);
        escrow.lockTaskEscrow(keccak256("exact"), worker, PER_TASK, block.number + 10, keccak256("p"));
        assertEq(escrow.getTask(keccak256("exact")).amount, PER_TASK);
    }

    function testRefundAfterDeadlineRestoresPrincipal() public {
        bytes32 taskId = keccak256("refund-me");
        uint256 deadline = block.number + 50;
        uint256 before = token.balanceOf(principal);

        vm.prank(agent);
        escrow.lockTaskEscrow(taskId, worker, FEE, deadline, keccak256("payload"));

        vm.expectRevert(INexusEscrow.DeadlineNotPassed.selector);
        escrow.refundExpiredTask(taskId);

        vm.roll(deadline);
        vm.expectRevert(INexusEscrow.DeadlineNotPassed.selector);
        escrow.refundExpiredTask(taskId);

        vm.roll(deadline + 1);
        vm.prank(address(0xBEEF));
        escrow.refundExpiredTask(taskId);

        TaskEscrow memory task = escrow.getTask(taskId);
        assertEq(uint8(task.status), uint8(EscrowStatus.REFUNDED));
        assertEq(token.balanceOf(principal), before);
        assertEq(token.balanceOf(address(escrow)), 0);
        assertEq(token.balanceOf(worker), 0);
    }

    function testCannotRefundOrReplayCompletedTask() public {
        bytes32 taskId = keccak256("done");
        vm.prank(agent);
        escrow.lockTaskEscrow(taskId, worker, FEE, block.number + 5, keccak256("p"));
        escrow.completeTaskWithProof(taskId, keccak256("r"), _signProof(WORKER_PK, taskId, keccak256("r")));

        vm.roll(block.number + 10);
        vm.expectRevert(INexusEscrow.TaskNotPending.selector);
        escrow.refundExpiredTask(taskId);

        vm.expectRevert(INexusEscrow.TaskNotPending.selector);
        escrow.completeTaskWithProof(taskId, keccak256("r2"), _signProof(WORKER_PK, taskId, keccak256("r2")));
    }

    function testMissingTaskAndDuplicateId() public {
        vm.expectRevert(INexusEscrow.TaskNotFound.selector);
        escrow.refundExpiredTask(keccak256("missing"));

        vm.expectRevert(INexusEscrow.TaskNotFound.selector);
        escrow.completeTaskWithProof(keccak256("missing"), keccak256("r"), _signProof(WORKER_PK, keccak256("missing"), keccak256("r")));

        bytes32 taskId = keccak256("dup");
        vm.prank(agent);
        escrow.lockTaskEscrow(taskId, worker, FEE, block.number + 10, keccak256("p"));
        vm.prank(agent);
        vm.expectRevert(INexusEscrow.TaskNotPending.selector);
        escrow.lockTaskEscrow(taskId, worker, FEE, block.number + 10, keccak256("p"));
    }

    function testExpiredSessionCannotLock() public {
        vm.warp(block.timestamp + 1 days);
        vm.prank(agent);
        vm.expectRevert(INexusEscrow.SessionExpired.selector);
        escrow.lockTaskEscrow(keccak256("late"), worker, FEE, block.number + 10, keccak256("p"));
    }

    function testEmergencyCircuitBreakerFreezesAgent() public {
        vm.expectEmit(true, true, false, true, address(escrow));
        emit SessionRevoked(principal, agent);
        vm.prank(principal);
        escrow.emergencyRevokeSession(agent);

        assertFalse(escrow.getSession(principal, agent).isActive);

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.SessionInactive.selector);
        escrow.lockTaskEscrow(keccak256("frozen"), worker, FEE, block.number + 10, keccak256("p"));

        vm.prank(principal);
        vm.expectRevert(INexusEscrow.SessionInactive.selector);
        escrow.emergencyRevokeSession(agent);

        vm.expectRevert(INexusEscrow.SessionInactive.selector);
        escrow.emergencyRevokeSession(agent);
    }

    function testReauthorizationClearsCircuitBreaker() public {
        vm.prank(principal);
        escrow.emergencyRevokeSession(agent);

        _authorize(agent, HOURLY, PER_TASK, block.timestamp + 2 days);
        assertTrue(escrow.getSession(principal, agent).isActive);

        vm.prank(agent);
        escrow.lockTaskEscrow(keccak256("thawed"), worker, FEE, block.number + 10, keccak256("p"));
        assertEq(uint8(escrow.getTask(keccak256("thawed")).status), uint8(EscrowStatus.PENDING));
    }

    function testUnknownAgentCannotLock() public {
        vm.prank(vm.addr(0xDEAD));
        vm.expectRevert(INexusEscrow.SessionInactive.selector);
        escrow.lockTaskEscrow(keccak256("stranger"), worker, FEE, block.number + 10, keccak256("p"));
    }

    function testAgentKeyCannotBeReboundToAnotherPrincipal() public {
        uint256 otherPk = 0x0711;
        address other = vm.addr(otherPk);
        token.mint(other, FEE);
        uint256 expiresAt = block.timestamp + 1 days;
        bytes memory sig = _signSession(escrow, otherPk, other, agent, HOURLY, PER_TASK, expiresAt, 0);
        vm.expectRevert(INexusEscrow.SessionInactive.selector);
        escrow.registerSessionWithSig(other, agent, HOURLY, PER_TASK, expiresAt, 0, sig);
    }

    function testReentrancyOnLockIsRejected() public {
        ReentrantTUSD8 evil = new ReentrantTUSD8();
        NexusEscrow guarded = new NexusEscrow(address(evil));
        address attacker = address(evil);
        evil.mint(principal, 10_000_000);
        vm.prank(principal);
        evil.approve(address(guarded), type(uint256).max);

        uint256 expiresAt = block.timestamp + 1 days;
        bytes memory sig =
            _signSession(guarded, PRINCIPAL_PK, principal, attacker, HOURLY, PER_TASK, expiresAt, 0);
        guarded.registerSessionWithSig(principal, attacker, HOURLY, PER_TASK, expiresAt, 0, sig);

        bytes32 outerTask = keccak256("outer");
        evil.arm(guarded, worker, outerTask, FEE);

        vm.expectRevert(ReentrancyGuard.ReentrancyGuardReentrantCall.selector);
        evil.trigger();
        assertEq(uint8(guarded.getTask(outerTask).status), uint8(EscrowStatus.NONE));
        assertEq(evil.balanceOf(address(guarded)), 0);
    }

    function _authorize(address agentKey, uint256 hourlyLimit, uint256 perTaskLimit, uint256 expiresAt)
        internal
    {
        uint256 nonce = escrow.nonces(principal);
        bytes memory sig = _signSession(
            escrow, PRINCIPAL_PK, principal, agentKey, hourlyLimit, perTaskLimit, expiresAt, nonce
        );
        escrow.registerSessionWithSig(
            principal, agentKey, hourlyLimit, perTaskLimit, expiresAt, nonce, sig
        );
    }

    function _signSession(
        NexusEscrow target,
        uint256 pk,
        address principal_,
        address agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt,
        uint256 nonce
    ) internal view returns (bytes memory) {
        bytes32 structHash = keccak256(
            abi.encode(
                target.SESSION_TYPEHASH(),
                principal_,
                agentKey,
                hourlyLimit,
                perTaskLimit,
                expiresAt,
                nonce
            )
        );
        bytes32 digest = keccak256(abi.encodePacked("\x19\x01", target.domainSeparator(), structHash));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(pk, digest);
        return abi.encodePacked(r, s, v);
    }

    function _signProof(uint256 pk, bytes32 taskId, bytes32 resultHash) internal pure returns (bytes memory) {
        bytes32 digest = keccak256(abi.encodePacked(taskId, resultHash));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(pk, digest);
        return abi.encodePacked(r, s, v);
    }
}
