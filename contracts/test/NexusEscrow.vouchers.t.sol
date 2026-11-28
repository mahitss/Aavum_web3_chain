// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";

import {NexusEscrow} from "../src/NexusEscrow.sol";
import {NexusIdentityRegistry} from "../src/NexusIdentityRegistry.sol";
import {MockERC20} from "../src/mock/MockERC20.sol";
import {INexusEscrow} from "../src/interfaces/INexusEscrow.sol";

contract NexusEscrowVoucherTest is Test {
    uint256 internal constant PRINCIPAL_PK = 0xA11CE;
    uint256 internal constant AGENT_PK = 0xA6E47;
    uint256 internal constant WORKER_PK = 0xB0B;
    uint256 internal constant DEPOSIT = 1_000_000;
    uint256 internal constant SETTLED = 500_000;

    address internal principal;
    address internal agent;
    address internal worker;
    MockERC20 internal token;
    NexusEscrow internal escrow;
    NexusIdentityRegistry internal passports;

    function setUp() public {
        principal = vm.addr(PRINCIPAL_PK);
        agent = vm.addr(AGENT_PK);
        worker = vm.addr(WORKER_PK);
        token = new MockERC20();
        escrow = new NexusEscrow(address(token));
        passports = new NexusIdentityRegistry();
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
                        uint256(2_000_000),
                        uint256(1_000_000),
                        expiresAt,
                        nonce
                    )
                )
            )
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(PRINCIPAL_PK, digest);
        escrow.registerSessionWithSig(principal, agent, 2_000_000, 1_000_000, expiresAt, v, r, s);
    }

    function test_RegisterAgentPassportOnce() public {
        vm.prank(worker);
        uint256 agentId = passports.registerAgent("https://worker.example/.well-known/nexus402.json");
        assertEq(agentId, 1);
        assertEq(passports.agentIdByWallet(worker), 1);
        assertTrue(passports.isRegisteredAgent(worker));
        assertEq(passports.ownerOf(1), worker);
        assertEq(passports.tokenURI(1), "https://worker.example/.well-known/nexus402.json");

        vm.prank(worker);
        vm.expectRevert(NexusIdentityRegistry.AgentAlreadyRegistered.selector);
        passports.registerAgent("https://worker.example/.well-known/nexus402.json");
    }

    function test_TenOffChainVouchersSettleInOneTransaction() public {
        uint256 principalBefore = token.balanceOf(principal);
        vm.prank(agent);
        bytes32 tabId = escrow.openSessionTab(principal, worker, DEPOSIT, 50);
        assertEq(escrow.getSession(principal, agent).spentInWindow, DEPOSIT);

        uint256 validBefore = block.timestamp + 1 hours;
        for (uint256 step = 1; step <= 10; step++) {
            _signVoucher(tabId, step * 50_000, step, validBefore);
        }
        bytes memory finalVoucher = _signVoucher(tabId, SETTLED, 10, validBefore);
        escrow.settleVoucherBatch(tabId, SETTLED, 10, validBefore, finalVoucher);

        assertEq(token.balanceOf(worker), SETTLED);
        assertEq(token.balanceOf(principal), principalBefore - DEPOSIT + (DEPOSIT - SETTLED));
        assertEq(token.balanceOf(address(escrow)), 0);
        assertEq(escrow.getSession(principal, agent).spentInWindow, SETTLED);
        assertFalse(escrow.getSessionTab(tabId).open);
        (uint256 completed,, uint256 volume,) = escrow.workerStats(worker);
        assertEq(completed, 1);
        assertEq(volume, SETTLED);
    }

    function test_RevertWhen_NonceIsStaleOrReplayed() public {
        vm.prank(agent);
        bytes32 tabId = escrow.openSessionTab(principal, worker, DEPOSIT, 50);
        uint256 validBefore = block.timestamp + 1 hours;
        bytes memory voucher = _signVoucher(tabId, 50_000, 0, validBefore);
        vm.expectRevert(INexusEscrow.StaleVoucherNonce.selector);
        escrow.settleVoucherBatch(tabId, 50_000, 0, validBefore, voucher);

        bytes memory first = _signVoucher(tabId, SETTLED, 10, validBefore);
        escrow.settleVoucherBatch(tabId, SETTLED, 10, validBefore, first);
        vm.expectRevert(INexusEscrow.TabClosed.selector);
        escrow.settleVoucherBatch(tabId, SETTLED, 10, validBefore, first);

        bytes memory older = _signVoucher(tabId, 50_000, 9, validBefore);
        vm.expectRevert(INexusEscrow.TabClosed.selector);
        escrow.settleVoucherBatch(tabId, 50_000, 9, validBefore, older);
    }

    function test_RevertWhen_WorkerIsNotAllowlisted() public {
        vm.prank(principal);
        escrow.setEnforceAllowlist(true);

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.WorkerNotAllowlisted.selector);
        escrow.lockTaskEscrow(principal, worker, 50_000, 50, keccak256("blocked"));

        vm.prank(agent);
        vm.expectRevert(INexusEscrow.WorkerNotAllowlisted.selector);
        escrow.openSessionTab(principal, worker, DEPOSIT, 50);

        vm.prank(principal);
        escrow.setWorkerAllowlist(worker, true);
        vm.prank(agent);
        escrow.openSessionTab(principal, worker, DEPOSIT, 50);
    }

    function _signVoucher(bytes32 tabId, uint256 cumulative, uint256 nonce, uint256 validBefore)
        internal
        view
        returns (bytes memory)
    {
        bytes32 digest = keccak256(
            abi.encodePacked(
                "\x19\x01",
                escrow.domainSeparator(),
                keccak256(abi.encode(escrow.PAYMENT_VOUCHER_TYPEHASH(), tabId, cumulative, nonce, validBefore))
            )
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(AGENT_PK, digest);
        return abi.encodePacked(r, s, v);
    }
}
