// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {MockERC20} from "../src/mock/MockERC20.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";
import {EscrowStatus} from "../src/types/NexusTypes.sol";

contract NexusEscrowEvaluatorTest is Test {
    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant WORKER_PK = 0xB0B;
    uint256 internal constant EVALUATOR_PK = 0xE7A1;
    uint256 internal constant FEE = 300_000;

    address internal principal;
    address internal agent;
    address internal worker;
    address internal evaluator;
    MockERC20 internal token;
    NexusEscrow internal escrow;

    function setUp() public {
        principal = vm.addr(PRINCIPAL_PK);
        agent = vm.addr(AGENT_PK);
        worker = vm.addr(WORKER_PK);
        evaluator = vm.addr(EVALUATOR_PK);
        token = new MockERC20();
        escrow = new NexusEscrow(address(token));
        token.mint(principal, 10_000_000);
        vm.prank(principal);
        token.approve(address(escrow), type(uint256).max);

        uint256 expiresAt = block.timestamp + 1 days;
        bytes32 digest = keccak256(
            abi.encodePacked(
                "\x19\x01",
                escrow.domainSeparator(),
                keccak256(
                    abi.encode(
                        escrow.SESSION_TYPEHASH(),
                        principal,
                        agent,
                        uint256(2_000_000),
                        uint256(1_000_000),
                        expiresAt,
                        escrow.nonces(principal)
                    )
                )
            )
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(PRINCIPAL_PK, digest);
        escrow.registerSessionWithSig(principal, agent, 2_000_000, 1_000_000, expiresAt, v, r, s);
    }

    function test_EvaluatedJobPaysWorkerAndRaisesReputation() public {
        bytes32 taskId = _lock();
        bytes32 resultHash = keccak256("accepted-result");
        escrow.submitWorkForEvaluation(taskId, resultHash, _workerSig(taskId, resultHash));
        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.SUBMITTED));

        bytes32 evaluationHash = keccak256("accept");
        escrow.evaluateTask(taskId, true, evaluationHash, _evaluatorSig(taskId, true, evaluationHash));

        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.COMPLETED));
        assertEq(token.balanceOf(worker), FEE);
        assertEq(token.balanceOf(principal), 10_000_000 - FEE);
        (,, uint256 volume,) = escrow.workerStats(worker);
        assertEq(volume, FEE);
        (, uint256 bps) = escrow.getWorkerReputation(worker);
        assertEq(bps, 10_000);
    }

    function test_GarbageOutputRefundsPrincipalAndSlashesReliability() public {
        bytes32 taskId = _lock();
        bytes32 resultHash = keccak256("garbage");
        escrow.submitWorkForEvaluation(taskId, resultHash, _workerSig(taskId, resultHash));

        bytes32 evaluationHash = keccak256("reject");
        escrow.evaluateTask(taskId, false, evaluationHash, _evaluatorSig(taskId, false, evaluationHash));

        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.REJECTED));
        assertEq(token.balanceOf(worker), 0);
        assertEq(token.balanceOf(principal), 10_000_000);
        assertEq(escrow.getSession(principal, agent).spentInWindow, 0);
        (, uint256 bps) = escrow.getWorkerReputation(worker);
        assertEq(bps, 0);
    }

    function test_RevertWhen_EvaluatorSignatureIsUnauthorized() public {
        bytes32 taskId = _lock();
        bytes32 resultHash = keccak256("result");
        escrow.submitWorkForEvaluation(taskId, resultHash, _workerSig(taskId, resultHash));
        bytes32 evaluationHash = keccak256("forged");

        vm.expectRevert(INexusEscrow.InvalidEvaluatorSignature.selector);
        escrow.evaluateTask(taskId, true, evaluationHash, _workerSig(taskId, resultHash));
        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.SUBMITTED));
    }

    function test_VoidAuthorizationReturnsFundsWithoutPenalty() public {
        bytes32 taskId = _lock();
        uint256 principalBefore = token.balanceOf(principal);

        vm.prank(worker);
        escrow.voidAuthorization(taskId);

        assertEq(uint8(escrow.getTask(taskId).status), uint8(EscrowStatus.REFUNDED));
        assertEq(token.balanceOf(principal), principalBefore + FEE);
        assertEq(escrow.getSession(principal, agent).spentInWindow, 0);
        (, uint256 refunded,,) = escrow.workerStats(worker);
        assertEq(refunded, 0);
        (, uint256 bps) = escrow.getWorkerReputation(worker);
        assertEq(bps, 10_000);

        vm.expectRevert(INexusEscrow.AuthorizationCaptured.selector);
        escrow.voidAuthorization(taskId);
    }

    function _lock() internal returns (bytes32 taskId) {
        vm.prank(agent);
        taskId = escrow.lockEvaluatedEscrow(principal, worker, evaluator, FEE, 50, 20, keccak256("evaluated"));
    }

    function _workerSig(bytes32 taskId, bytes32 resultHash) internal pure returns (bytes memory) {
        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(keccak256(abi.encodePacked(taskId, resultHash)));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(WORKER_PK, digest);
        return abi.encodePacked(r, s, v);
    }

    function _evaluatorSig(bytes32 taskId, bool approved, bytes32 evaluationHash) internal pure returns (bytes memory) {
        bytes32 digest = MessageHashUtils.toEthSignedMessageHash(
            keccak256(abi.encodePacked(taskId, approved, evaluationHash))
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(EVALUATOR_PK, digest);
        return abi.encodePacked(r, s, v);
    }
}
