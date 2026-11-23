// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {NexusEscrow} from "../src/NexusEscrow.sol";

/// @notice Deploys NexusEscrow against tUSD8 on QDay Aevum Testnet (chain id 44005).
contract DeployAevum is Script {
    address internal constant DEFAULT_TUSD8 = 0x456569838f1d43009A91ebA4263147Afd7233483;

    function run() external returns (NexusEscrow escrow) {
        uint256 deployerKey = vm.envUint("PRIVATE_KEY");
        address tusd8 = vm.envOr("TUSD8_ADDRESS", DEFAULT_TUSD8);

        vm.startBroadcast(deployerKey);
        escrow = new NexusEscrow(tusd8);
        vm.stopBroadcast();

        console2.log("NexusEscrow", address(escrow));
        console2.log("paymentToken", tusd8);
        console2.log("chainId", block.chainid);
        console2.log("rpc", "https://rpc-test.qday.info");
        console2.log("domainSeparator");
        console2.logBytes32(escrow.domainSeparator());
    }
}
