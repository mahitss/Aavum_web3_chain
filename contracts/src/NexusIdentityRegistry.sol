// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ERC721} from "@openzeppelin/contracts/token/ERC721/ERC721.sol";
import {ERC721URIStorage} from "@openzeppelin/contracts/token/ERC721/extensions/ERC721URIStorage.sol";

/// @title NexusIdentityRegistry
/// @notice ERC-8004 style passport that binds a worker wallet to its Nexus402 manifest.
/// @dev One passport per wallet. Reputation stays on the wallet in NexusEscrow, so a new token id
///      cannot be minted to hide an existing score.
contract NexusIdentityRegistry is ERC721URIStorage {
    uint256 private _nextId = 1;

    mapping(address => uint256) public agentIdByWallet;

    event AgentRegistered(uint256 indexed agentId, address indexed wallet, string manifestURI);

    error AgentAlreadyRegistered();
    error EmptyManifest();

    constructor() ERC721("Nexus402 Agent Passport", "N402-ID") {}

    function registerAgent(string calldata manifestURI) external returns (uint256 agentId) {
        if (agentIdByWallet[msg.sender] != 0) revert AgentAlreadyRegistered();
        if (bytes(manifestURI).length == 0) revert EmptyManifest();

        agentId = _nextId++;
        agentIdByWallet[msg.sender] = agentId;
        _safeMint(msg.sender, agentId);
        _setTokenURI(agentId, manifestURI);
        emit AgentRegistered(agentId, msg.sender, manifestURI);
    }

    function isRegisteredAgent(address wallet) external view returns (bool) {
        return agentIdByWallet[wallet] != 0;
    }
}
