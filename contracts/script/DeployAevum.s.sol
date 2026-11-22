// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {NexusEscrow} from "../src/NexusEscrow.sol";

/// @notice Deploys NexusEscrow against tUSD8 on QDay Aevum Testnet (chain id 44005).
contract DeployAevum is Script {
    address internal constant TUSD8 = 0x456569838f1d43009A91ebA4263147Afd7233483;
    uint256 internal constant AEVUM_CHAIN_ID = 44005;

    function run() external returns (NexusEscrow escrow) {
        require(block.chainid == AEVUM_CHAIN_ID, "DeployAevum: chain must be Aevum 44005");

        uint256 deployerKey = vm.envUint("DEPLOYER_PRIVATE_KEY");
        vm.startBroadcast(deployerKey);
        escrow = new NexusEscrow(TUSD8);
        vm.stopBroadcast();

        console2.log("NexusEscrow", address(escrow));
        console2.log("tUSD8", TUSD8);
        console2.log("chainId", block.chainid);
    }
}
