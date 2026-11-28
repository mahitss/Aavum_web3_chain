// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {MockERC20} from "../src/mock/MockERC20.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";

contract NexusEscrowOrgVaultTest is Test {
    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant AGENT_TWO_PK = 0xA6E48;
    uint256 internal constant WORKER_PK = 0xB0B;

    address internal principal;
    address internal agent;
    address internal agentTwo;
    address internal worker;
    MockERC20 internal token;
    NexusEscrow internal escrow;

    function setUp() public {
        principal = vm.addr(PRINCIPAL_PK);
        agent = vm.addr(AGENT_PK);
        agentTwo = vm.addr(AGENT_TWO_PK);
        worker = vm.addr(WORKER_PK);
        token = new MockERC20();
        escrow = new NexusEscrow(address(token));
        token.mint(principal, 10_000_000);
        vm.prank(principal);
        token.approve(address(escrow), type(uint256).max);
        _authorize(AGENT_PK, agent);
        _authorize(AGENT_TWO_PK, agentTwo);
        vm.prank(principal);
        escrow.setOrgVaultLimits(500_000);
    }

    function test_TwoAgentsShareOneGlobalHourlyLimit() public {
        vm.prank(agent);
        escrow.lockTaskEscrow(principal, worker, 300_000, 50, keccak256("agent-1"));
        vm.prank(agentTwo);
        escrow.lockTaskEscrow(principal, worker, 200_000, 50, keccak256("agent-2"));

        assertEq(escrow.getOrgVault(principal).globalSpentInWindow, 500_000);
        assertEq(escrow.getSession(principal, agent).spentInWindow, 300_000);
        assertEq(escrow.getSession(principal, agentTwo).spentInWindow, 200_000);
    }

    function test_RevertWhen_GlobalHourlyLimitOverflows() public {
        vm.prank(agent);
        escrow.lockTaskEscrow(principal, worker, 300_000, 50, keccak256("first"));
        vm.prank(agentTwo);
        vm.expectRevert(INexusEscrow.ExceedsGlobalHourlyLimit.selector);
        escrow.lockTaskEscrow(principal, worker, 300_000, 50, keccak256("overflow"));
        assertEq(escrow.getOrgVault(principal).globalSpentInWindow, 300_000);
    }

    function test_EmergencyPauseBlocksEveryActiveSession() public {
        vm.prank(principal);
        escrow.emergencyPauseAllAgents();

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.OrgVaultPaused.selector);
        escrow.lockTaskEscrow(principal, worker, 50_000, 50, keccak256("paused-1"));

        vm.prank(agentTwo);
        vm.expectRevert(INexusEscrow.OrgVaultPaused.selector);
        escrow.lockEvaluatedEscrow(principal, worker, worker, 50_000, 50, 10, keccak256("paused-2"));

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.OrgVaultPaused.selector);
        escrow.openSessionTab(principal, worker, 50_000, 50);

        vm.prank(principal);
        escrow.unpauseAllAgents();
        vm.prank(agentTwo);
        escrow.lockTaskEscrow(principal, worker, 50_000, 50, keccak256("resumed"));
        assertEq(escrow.getOrgVault(principal).globalSpentInWindow, 50_000);
        assertFalse(escrow.getOrgVault(principal).globalPaused);
    }

    function _authorize(uint256 pk, address agentKey) internal {
        uint256 expiresAt = block.timestamp + 1 days;
        bytes32 digest = keccak256(
            abi.encodePacked(
                "\x19\x01",
                escrow.domainSeparator(),
                keccak256(
                    abi.encode(
                        escrow.SESSION_TYPEHASH(),
                        principal,
                        agentKey,
                        uint256(1_000_000),
                        uint256(1_000_000),
                        expiresAt,
                        escrow.nonces(principal)
                    )
                )
            )
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(PRINCIPAL_PK, digest);
        escrow.registerSessionWithSig(principal, agentKey, 1_000_000, 1_000_000, expiresAt, v, r, s);
    }
}
