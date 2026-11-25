// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {MockERC20} from "../src/mock/MockERC20.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";
import {WorkerStats} from "../src/types/NexusTypes.sol";

contract NexusEscrowReputationTest is Test {
    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant WORKER_PK = 0xB0B;
    uint256 internal constant FEE = 100_000;
    uint256 internal constant ACTUAL = 40_000;

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

        uint256 expiresAt = block.timestamp + 1 days;
        uint256 nonce = escrow.nonces(principal);
        bytes32 digest = keccak256(
            abi.encodePacked(
                "\x19\x01",
                escrow.domainSeparator(),
                keccak256(
                    abi.encode(
                        escrow.SESSION_TYPEHASH(),
                        principal,
                        agent,
                        uint256(1_000_000),
                        uint256(500_000),
                        expiresAt,
                        nonce
                    )
                )
            )
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(PRINCIPAL_PK, digest);
        escrow.registerSessionWithSig(principal, agent, 1_000_000, 500_000, expiresAt, v, r, s);
    }

    function test_UnratedWorkerReadsFullReliability() public view {
        (WorkerStats memory stats, uint256 bps) = escrow.getWorkerReputation(worker);
        assertEq(stats.tasksCompleted, 0);
        assertEq(stats.tasksRefunded, 0);
        assertEq(bps, 10_000);
    }

    function test_CompletionAndRefundMoveReliability() public {
        bytes32 completed = _lock("done");
        bytes32 resultHash = keccak256("proof");
        escrow.completeTaskWithProof(completed, resultHash, _signProof(completed, resultHash));

        (WorkerStats memory afterComplete, uint256 completeBps) = escrow.getWorkerReputation(worker);
        assertEq(afterComplete.tasksCompleted, 1);
        assertEq(afterComplete.tasksRefunded, 0);
        assertEq(afterComplete.totalVolumeSettled, FEE);
        assertEq(afterComplete.lastActiveBlock, block.number);
        assertEq(completeBps, 10_000);

        bytes32 expired = _lock("late");
        vm.roll(block.number + 51);
        escrow.refundExpiredTask(expired);

        (WorkerStats memory afterRefund, uint256 refundBps) = escrow.getWorkerReputation(worker);
        assertEq(afterRefund.tasksCompleted, 1);
        assertEq(afterRefund.tasksRefunded, 1);
        assertEq(afterRefund.totalVolumeSettled, FEE);
        assertEq(afterRefund.lastActiveBlock, block.number);
        assertEq(refundBps, 5_000);
    }

    function test_MeteredSettlementCountsActualVolume() public {
        bytes32 taskId = _lock("meter");
        bytes32 resultHash = keccak256("usage");
        escrow.completeTaskMetered(taskId, ACTUAL, resultHash, _signMetered(taskId, ACTUAL, resultHash));

        (WorkerStats memory stats, uint256 bps) = escrow.getWorkerReputation(worker);
        assertEq(stats.tasksCompleted, 1);
        assertEq(stats.totalVolumeSettled, ACTUAL);
        assertEq(bps, 10_000);
    }

    function _lock(string memory label) internal returns (bytes32 taskId) {
        vm.prank(agent);
        taskId = escrow.lockTaskEscrow(principal, worker, FEE, 50, keccak256(bytes(label)));
    }

    function _signProof(bytes32 taskId, bytes32 resultHash) internal pure returns (bytes memory) {
        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, resultHash)));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(WORKER_PK, digest);
        return abi.encodePacked(r, s, v);
    }

    function _signMetered(bytes32 taskId, uint256 actualAmount, bytes32 resultHash)
        internal
        pure
        returns (bytes memory)
    {
        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(
            keccak256(abi.encodePacked(taskId, actualAmount, resultHash))
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(WORKER_PK, digest);
        return abi.encodePacked(r, s, v);
    }
}
