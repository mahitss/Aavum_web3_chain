// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {MockERC20} from "../src/mock/MockERC20.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";
import {SessionConfig} from "../src/types/NexusTypes.sol";

contract NexusEscrowMeteredTest is Test {
    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant WORKER_PK = 0xB0B;
    uint256 internal constant CEILING = 500_000;
    uint256 internal constant ACTUAL = 180_000;
    uint256 internal constant CHANGE = 320_000;

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
                        escrow.SESSION_TYPEHASH(), principal, agent, uint256(1_000_000), CEILING, expiresAt, nonce
                    )
                )
            )
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(PRINCIPAL_PK, digest);
        escrow.registerSessionWithSig(principal, agent, 1_000_000, CEILING, expiresAt, v, r, s);
    }

    function test_MeteredSplitReturnsChangeAndCreditsWindow() public {
        uint256 principalBefore = token.balanceOf(principal);
        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, CEILING, 50, keccak256("upto"));
        assertEq(escrow.getSession(principal, agent).spentInWindow, CEILING);

        bytes32 resultHash = keccak256("metered");
        escrow.completeTaskMetered(taskId, ACTUAL, resultHash, _signMetered(taskId, ACTUAL, resultHash));

        assertEq(token.balanceOf(worker), ACTUAL);
        assertEq(token.balanceOf(principal), principalBefore - CEILING + CHANGE);
        assertEq(token.balanceOf(address(escrow)), 0);
        SessionConfig memory session = escrow.getSession(principal, agent);
        assertEq(session.spentInWindow, ACTUAL);
        assertEq(escrow.getTask(taskId).resultHash, resultHash);
    }

    function test_RevertWhen_ActualExceedsLockedCeiling() public {
        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, CEILING, 50, keccak256("over"));
        bytes32 resultHash = keccak256("too-much");
        vm.expectRevert(INexusEscrow.ExceedsEscrowAmount.selector);
        escrow.completeTaskMetered(taskId, CEILING + 1, resultHash, _signMetered(taskId, CEILING + 1, resultHash));
    }

    function test_RevertWhen_SignatureUsesDifferentAmount() public {
        vm.prank(agent);
        bytes32 taskId = escrow.lockTaskEscrow(principal, worker, CEILING, 50, keccak256("tamper"));
        bytes32 resultHash = keccak256("result");
        bytes memory tampered = _signMetered(taskId, ACTUAL - 1, resultHash);
        vm.expectRevert(INexusEscrow.InvalidProofSignature.selector);
        escrow.completeTaskMetered(taskId, ACTUAL, resultHash, tampered);
        assertEq(token.balanceOf(worker), 0);
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
