// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {MockERC20} from "../src/mock/MockERC20.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";
import {EscrowStatus, SessionConfig, TaskEscrow} from "../src/types/NexusTypes.sol";

contract NexusEscrowTest is Test {
    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant WORKER_PK = 0xB0B;
    uint256 internal constant HOURLY = 500_000;
    uint256 internal constant PER_TASK = 500_000;
    uint256 internal constant FEE = 500_000;

    address internal principal;
    address internal agent;
    address internal worker;
    MockERC20 internal token;
    NexusEscrow internal escrow;

    function setUp() public {
        principal = vm.addr(PRINCIPAL_PK);
        agent = vm.addr(AGENT_PK);
        worker = vm.addr(WORKER_PK);

        token = new MockERC20();
        escrow = new NexusEscrow(address(token));
        token.mint(principal, 10_000_000);

        vm.prank(principal);
        token.approve(address(escrow), type(uint256).max);
    }

    function test_RegisterSessionWithValidSig() public {
        uint256 expiresAt = block.timestamp + 1 days;
        (uint8 v, bytes32 r, bytes32 s) =
            _signSession(PRINCIPAL_PK, principal, agent, HOURLY, PER_TASK, expiresAt);

        escrow.registerSessionWithSig(principal, agent, HOURLY, PER_TASK, expiresAt, v, r, s);

        SessionConfig memory session = escrow.getSession(principal, agent);
        assertEq(session.agentKey, agent);
        assertEq(session.hourlyLimit, HOURLY);
        assertEq(session.perTaskLimit, PER_TASK);
        assertEq(session.expiresAt, expiresAt);
        assertEq(session.currentWindow, block.timestamp / 3600);
        assertEq(session.spentInWindow, 0);
        assertTrue(session.isActive);
        assertEq(escrow.nonces(principal), 1);
        assertEq(address(escrow.paymentToken()), address(token));
    }

    function test_RevertWhen_InvalidSignature() public {
        uint256 expiresAt = block.timestamp + 1 days;
        (uint8 v, bytes32 r, bytes32 s) =
            _signSession(AGENT_PK, principal, agent, HOURLY, PER_TASK, expiresAt);

        vm.expectRevert(INexusEscrow.InvalidSignature.selector);
        escrow.registerSessionWithSig(principal, agent, HOURLY, PER_TASK, expiresAt, v, r, s);
        assertFalse(escrow.getSession(principal, agent).isActive);
        assertEq(escrow.nonces(principal), 0);
    }

    function test_RevertWhen_ExceedingPerTaskLimit() public {
        _authorizeDefault();

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.ExceedsPerTaskLimit.selector);
        escrow.lockTaskEscrow(principal, worker, PER_TASK + 1, 50, keccak256("over"));
    }

    function test_RollingHourlyVelocityCap() public {
        _authorizeDefault();

        vm.prank(agent);
        escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256("hour-1"));

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.ExceedsHourlyLimit.selector);
        escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256("hour-2"));

        vm.warp(block.timestamp + 3601);

        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256("hour-3"));

        SessionConfig memory session = escrow.getSession(principal, agent);
        assertEq(session.currentWindow, block.timestamp / 3600);
        assertEq(session.spentInWindow, FEE);
        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.PENDING));
    }

    function test_EndToEnd_LockAndCompleteProof() public {
        _authorizeDefault();
        uint256 principalBefore = token.balanceOf(principal);

        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256("payload"));

        assertEq(token.balanceOf(address(escrow)), FEE);
        assertEq(token.balanceOf(principal), principalBefore - FEE);
        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.PENDING));

        bytes32 resultHash = keccak256("result");
        escrow.completeTaskWithProof(taskId, resultHash, _signProof(WORKER_PK, taskId, resultHash));

        TaskEscrow memory done = escrow.getTask(taskId);
        assertEq(uint8(done.status), uint8(EscrowStatus.COMPLETED));
        assertEq(done.resultHash, resultHash);
        assertEq(done.worker, worker);
        assertEq(token.balanceOf(worker), FEE);
        assertEq(token.balanceOf(address(escrow)), 0);
    }

    function test_RevertWhen_UnauthorizedWorkerClaims() public {
        _authorizeDefault();
        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256("payload"));

        bytes32 resultHash = keccak256("result");
        bytes memory forged = _signProof(AGENT_PK, taskId, resultHash);

        vm.expectRevert(INexusEscrow.InvalidProofSignature.selector);
        escrow.completeTaskWithProof(taskId, resultHash, forged);

        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.PENDING));
        assertEq(token.balanceOf(worker), 0);
        assertEq(token.balanceOf(address(escrow)), FEE);
    }

    function test_RefundExpiredTask() public {
        _authorizeDefault();
        uint256 before = token.balanceOf(principal);

        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256("refund"));
        uint256 deadline = escrow.getTask(taskId).deadline;

        vm.roll(deadline);
        vm.expectRevert(INexusEscrow.DeadlineNotPassed.selector);
        escrow.refundExpiredTask(taskId);

        vm.roll(deadline + 1);
        escrow.refundExpiredTask(taskId);

        TaskEscrow memory task = escrow.getTask(taskId);
        assertEq(uint8(task.status), uint8(EscrowStatus.REFUNDED));
        assertEq(token.balanceOf(principal), before);
        assertEq(token.balanceOf(address(escrow)), 0);
        assertEq(token.balanceOf(worker), 0);
    }

    function test_EmergencyRevokeCircuitBreaker() public {
        _authorizeDefault();

        vm.prank(principal);
        escrow.emergencyRevokeSession(agent);
        assertFalse(escrow.getSession(principal, agent).isActive);

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.SessionInactive.selector);
        escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256("frozen"));
    }

    function _authorizeDefault() internal {
        uint256 expiresAt = block.timestamp + 1 days;
        (uint8 v, bytes32 r, bytes32 s) =
            _signSession(PRINCIPAL_PK, principal, agent, HOURLY, PER_TASK, expiresAt);
        escrow.registerSessionWithSig(principal, agent, HOURLY, PER_TASK, expiresAt, v, r, s);
    }

    function _signSession(
        uint256 pk,
        address principal_,
        address agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt
    ) internal view returns (uint8 v, bytes32 r, bytes32 s) {
        uint256 nonce = escrow.nonces(principal_);
        bytes32 digest = _sessionDigest(principal_, agentKey, hourlyLimit, perTaskLimit, expiresAt, nonce);
        (v, r, s) = vm.sign(pk, digest);
    }

    function _sessionDigest(
        address principal_,
        address agentKey,
        uint256 hourlyLimit,
        uint256 perTaskLimit,
        uint256 expiresAt,
        uint256 nonce
    ) internal view returns (bytes32) {
        bytes32 typehash = escrow.SESSION_TYPEHASH();
        bytes32 structHash = keccak256(
            abi.encode(typehash, principal_, agentKey, hourlyLimit, perTaskLimit, expiresAt, nonce)
        );
        return keccak256(abi.encodePacked("\x19\x01", escrow.domainSeparator(), structHash));
    }

    function _signProof(uint256 pk, bytes32 taskId, bytes32 resultHash) internal pure returns (bytes memory) {
        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(
            keccak256(abi.encodePacked(taskId, resultHash))
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(pk, digest);
        return abi.encodePacked(r, s, v);
    }
}
