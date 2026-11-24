// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";
import {EscrowStatus, SessionConfig} from "../src/types/NexusTypes.sol";

contract ReenterToken is ERC20 {
    NexusEscrow public escrow;
    bool public attackClaim;
    bool public attackRefund;
    bytes32 public taskId;
    bytes32 public resultHash;
    bytes public sig;

    constructor() ERC20("Reenter", "RE") {}

    function decimals() public pure override returns (uint8) {
        return 6;
    }

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }

    function arm(NexusEscrow target, bytes32 id, bytes32 hash, bytes calldata signature, bool refund) external {
        escrow = target;
        taskId = id;
        resultHash = hash;
        sig = signature;
        attackClaim = !refund;
        attackRefund = refund;
    }

    function _update(address from, address to, uint256 value) internal override {
        super._update(from, to, value);
        if (from != address(escrow)) return;
        if (attackClaim) {
            attackClaim = false;
            escrow.completeTaskWithProof(taskId, resultHash, sig);
        } else if (attackRefund) {
            attackRefund = false;
            escrow.refundExpiredTask(taskId);
        }
    }
}

contract NexusEscrowFuzzTest is Test {
    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant WORKER_PK = 0xB0B;
    uint256 internal constant HOURLY = 1_000_000;
    uint256 internal constant PER_TASK = 100_000;

    address internal principal;
    address internal agent;
    address internal worker;
    ReenterToken internal token;
    NexusEscrow internal escrow;

    function setUp() public {
        principal = vm.addr(PRINCIPAL_PK);
        agent = vm.addr(AGENT_PK);
        worker = vm.addr(WORKER_PK);
        token = new ReenterToken();
        escrow = new NexusEscrow(address(token));
        token.mint(principal, 100_000_000);
        vm.prank(principal);
        token.approve(address(escrow), type(uint256).max);
        _authorize(HOURLY, PER_TASK);
    }

    function testFuzz_LockRespectsCaps(uint256 amount, uint32 durationBlocks) public {
        amount = bound(amount, 0, PER_TASK * 2);
        durationBlocks = uint32(bound(durationBlocks, 1, 5_000));

        vm.prank(agent);
        if (amount == 0) {
            vm.expectRevert(INexusEscrow.ZeroAmount.selector);
            escrow.lockTaskEscrow(principal, worker, amount, durationBlocks, keccak256("fuzz"));
            return;
        }
        if (amount > PER_TASK) {
            vm.expectRevert(INexusEscrow.ExceedsPerTaskLimit.selector);
            escrow.lockTaskEscrow(principal, worker, amount, durationBlocks, keccak256("fuzz"));
            return;
        }

        escrow.lockTaskEscrow(principal, worker, amount, durationBlocks, keccak256("fuzz"));
        SessionConfig memory session = escrow.getSession(principal, agent);
        assertLe(session.spentInWindow, session.hourlyLimit);
        assertLe(amount, session.perTaskLimit);
        assertEq(session.currentWindow, block.timestamp / 3600);
    }

    function test_ReplaySameNonceReverts() public {
        uint256 expiresAt = block.timestamp + 1 days;
        (uint8 v, bytes32 r, bytes32 s) = _sign(principal, agent, HOURLY, PER_TASK, expiresAt, 0, block.chainid);
        vm.expectRevert(INexusEscrow.InvalidSignature.selector);
        escrow.registerSessionWithSig(principal, agent, HOURLY, PER_TASK, expiresAt, v, r, s);
    }

    function testFuzz_WrongChainIdReverts(uint64 chainId) public {
        vm.assume(chainId != block.chainid);
        uint256 expiresAt = block.timestamp + 1 days;
        address freshAgent = vm.addr(0xF00D);
        (uint8 v, bytes32 r, bytes32 s) =
            _sign(principal, freshAgent, HOURLY, PER_TASK, expiresAt, escrow.nonces(principal), chainId);
        vm.expectRevert(INexusEscrow.InvalidSignature.selector);
        escrow.registerSessionWithSig(principal, freshAgent, HOURLY, PER_TASK, expiresAt, v, r, s);
    }

    function test_ReentrancyOnClaimIsBlocked() public {
        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, PER_TASK, 50, keccak256("reenter-claim"));
        bytes32 resultHash = keccak256("result");
        bytes memory sig = _proof(taskId, resultHash);
        token.arm(escrow, taskId, resultHash, sig, false);

        vm.expectRevert(ReentrancyGuard.ReentrancyGuardReentrantCall.selector);
        escrow.completeTaskWithProof(taskId, resultHash, sig);
        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.PENDING));
    }

    function test_ReentrancyOnRefundIsBlocked() public {
        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, PER_TASK, 1, keccak256("reenter-refund"));
        vm.roll(block.number + 2);
        token.arm(escrow, taskId, bytes32(0), "", true);

        vm.expectRevert(ReentrancyGuard.ReentrancyGuardReentrantCall.selector);
        escrow.refundExpiredTask(taskId);
        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.PENDING));
    }

    function test_RefundTwiceAndCompletedTaskRevert() public {
        vm.prank(agent);
        bytes32 refundId = escrow.lockTaskEscrow(principal, worker, 10_000, 1, keccak256("refund-once"));
        vm.roll(block.number + 2);
        escrow.refundExpiredTask(refundId);
        vm.expectRevert(INexusEscrow.TaskNotPending.selector);
        escrow.refundExpiredTask(refundId);

        vm.prank(agent);
        bytes32 doneId = escrow.lockTaskEscrow(principal, worker, 10_000, 50, keccak256("done"));
        bytes32 resultHash = keccak256("ok");
        escrow.completeTaskWithProof(doneId, resultHash, _proof(doneId, resultHash));
        vm.roll(block.number + 100);
        vm.expectRevert(INexusEscrow.TaskNotPending.selector);
        escrow.refundExpiredTask(doneId);
    }

    function _authorize(uint256 hourly, uint256 perTask) internal {
        uint256 expiresAt = block.timestamp + 1 days;
        (uint8 v, bytes32 r, bytes32 s) =
            _sign(principal, agent, hourly, perTask, expiresAt, escrow.nonces(principal), block.chainid);
        escrow.registerSessionWithSig(principal, agent, hourly, perTask, expiresAt, v, r, s);
    }

    function _sign(
        address principal_,
        address agentKey,
        uint256 hourly,
        uint256 perTask,
        uint256 expiresAt,
        uint256 nonce,
        uint256 chainId
    ) internal view returns (uint8 v, bytes32 r, bytes32 s) {
        bytes32 typehash = escrow.SESSION_TYPEHASH();
        bytes32 structHash = keccak256(abi.encode(typehash, principal_, agentKey, hourly, perTask, expiresAt, nonce));
        bytes32 domain = keccak256(
            abi.encode(
                keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
                keccak256("Nexus402 Protocol"),
                keccak256("1"),
                chainId,
                address(escrow)
            )
        );
        return vm.sign(PRINCIPAL_PK, keccak256(abi.encodePacked("\x19\x01", domain, structHash)));
    }

    function _proof(bytes32 taskId, bytes32 resultHash) internal pure returns (bytes memory) {
        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, resultHash)));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(WORKER_PK, digest);
        return abi.encodePacked(r, s, v);
    }
}
