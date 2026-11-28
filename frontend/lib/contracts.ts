import { isAddress, zeroAddress } from 'viem';

export const TUSD8_ADDRESS = '0x456569838f1d43009A91ebA4263147Afd7233483' as const;

const configured = process.env.NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS ?? '';
export const NEXUS_ESCROW_ADDRESS =
  isAddress(configured) && configured.toLowerCase() !== zeroAddress ? configured : undefined;

export const API_URL =
  process.env.NEXT_PUBLIC_ORCHESTRATOR_API_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  'http://localhost:8081';

export const nexusEscrowAbi = [
  {
    type: 'function',
    name: 'registerSessionWithSig',
    stateMutability: 'nonpayable',
    inputs: [
      { name: 'principal', type: 'address' },
      { name: 'agentKey', type: 'address' },
      { name: 'hourlyLimit', type: 'uint256' },
      { name: 'perTaskLimit', type: 'uint256' },
      { name: 'expiresAt', type: 'uint256' },
      { name: 'v', type: 'uint8' },
      { name: 'r', type: 'bytes32' },
      { name: 's', type: 'bytes32' },
    ],
    outputs: [],
  },
  {
    type: 'function',
    name: 'refundExpiredTask',
    stateMutability: 'nonpayable',
    inputs: [{ name: 'taskId', type: 'bytes32' }],
    outputs: [],
  },
  {
    type: 'function',
    name: 'emergencyRevokeSession',
    stateMutability: 'nonpayable',
    inputs: [{ name: 'agentKey', type: 'address' }],
    outputs: [],
  },
  {
    type: 'function',
    name: 'getSession',
    stateMutability: 'view',
    inputs: [
      { name: 'principal', type: 'address' },
      { name: 'agentKey', type: 'address' },
    ],
    outputs: [
      { name: 'agentKey', type: 'address' },
      { name: 'hourlyLimit', type: 'uint256' },
      { name: 'perTaskLimit', type: 'uint256' },
      { name: 'expiresAt', type: 'uint256' },
      { name: 'currentWindow', type: 'uint256' },
      { name: 'spentInWindow', type: 'uint256' },
      { name: 'isActive', type: 'bool' },
    ],
  },
  {
    type: 'function',
    name: 'getTask',
    stateMutability: 'view',
    inputs: [{ name: 'taskId', type: 'bytes32' }],
    outputs: [
      { name: 'taskId', type: 'bytes32' },
      { name: 'principal', type: 'address' },
      { name: 'agentKey', type: 'address' },
      { name: 'worker', type: 'address' },
      { name: 'amount', type: 'uint256' },
      { name: 'deadline', type: 'uint256' },
      { name: 'taskHash', type: 'bytes32' },
      { name: 'resultHash', type: 'bytes32' },
      { name: 'status', type: 'uint8' },
      { name: 'evaluator', type: 'address' },
      { name: 'challengeWindowBlocks', type: 'uint256' },
    ],
  },
  {
    type: 'function',
    name: 'nonces',
    stateMutability: 'view',
    inputs: [{ name: 'principal', type: 'address' }],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'event',
    name: 'SessionAuthorized',
    inputs: [
      { name: 'principal', type: 'address', indexed: true },
      { name: 'agentKey', type: 'address', indexed: true },
      { name: 'hourlyLimit', type: 'uint256', indexed: false },
      { name: 'perTaskLimit', type: 'uint256', indexed: false },
      { name: 'expiresAt', type: 'uint256', indexed: false },
    ],
  },
  {
    type: 'event',
    name: 'SessionRevoked',
    inputs: [
      { name: 'principal', type: 'address', indexed: true },
      { name: 'agentKey', type: 'address', indexed: true },
      { name: 'reason', type: 'string', indexed: false },
    ],
  },
  {
    type: 'event',
    name: 'TaskEscrowLocked',
    inputs: [
      { name: 'taskId', type: 'bytes32', indexed: true },
      { name: 'principal', type: 'address', indexed: true },
      { name: 'worker', type: 'address', indexed: true },
      { name: 'amount', type: 'uint256', indexed: false },
      { name: 'deadline', type: 'uint256', indexed: false },
    ],
  },
  {
    type: 'event',
    name: 'TaskCompleted',
    inputs: [
      { name: 'taskId', type: 'bytes32', indexed: true },
      { name: 'worker', type: 'address', indexed: true },
      { name: 'resultHash', type: 'bytes32', indexed: false },
      { name: 'amount', type: 'uint256', indexed: false },
    ],
  },
  {
    type: 'event',
    name: 'TaskRefunded',
    inputs: [
      { name: 'taskId', type: 'bytes32', indexed: true },
      { name: 'principal', type: 'address', indexed: true },
      { name: 'amount', type: 'uint256', indexed: false },
    ],
  },
  {
    type: 'function',
    name: 'enforceAllowlist',
    stateMutability: 'view',
    inputs: [{ name: 'principal', type: 'address' }],
    outputs: [{ name: '', type: 'bool' }],
  },
  {
    type: 'function',
    name: 'setEnforceAllowlist',
    stateMutability: 'nonpayable',
    inputs: [{ name: 'enabled', type: 'bool' }],
    outputs: [],
  },
  {
    type: 'function',
    name: 'setWorkerAllowlist',
    stateMutability: 'nonpayable',
    inputs: [
      { name: 'worker', type: 'address' },
      { name: 'allowed', type: 'bool' },
    ],
    outputs: [],
  },
  {
    type: 'function',
    name: 'setOrgVaultLimits',
    stateMutability: 'nonpayable',
    inputs: [{ name: 'globalHourlyLimit', type: 'uint256' }],
    outputs: [],
  },
  {
    type: 'function',
    name: 'emergencyPauseAllAgents',
    stateMutability: 'nonpayable',
    inputs: [],
    outputs: [],
  },
  {
    type: 'function',
    name: 'unpauseAllAgents',
    stateMutability: 'nonpayable',
    inputs: [],
    outputs: [],
  },
  {
    type: 'function',
    name: 'getOrgVault',
    stateMutability: 'view',
    inputs: [{ name: 'principal', type: 'address' }],
    outputs: [
      {
        type: 'tuple',
        components: [
          { name: 'globalHourlyLimit', type: 'uint256' },
          { name: 'globalCurrentWindow', type: 'uint256' },
          { name: 'globalSpentInWindow', type: 'uint256' },
          { name: 'globalPaused', type: 'bool' },
        ],
      },
    ],
  },
] as const;

export const erc20Abi = [
  {
    type: 'function',
    name: 'approve',
    stateMutability: 'nonpayable',
    inputs: [
      { name: 'spender', type: 'address' },
      { name: 'amount', type: 'uint256' },
    ],
    outputs: [{ name: '', type: 'bool' }],
  },
  {
    type: 'function',
    name: 'allowance',
    stateMutability: 'view',
    inputs: [
      { name: 'owner', type: 'address' },
      { name: 'spender', type: 'address' },
    ],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'balanceOf',
    stateMutability: 'view',
    inputs: [{ name: 'account', type: 'address' }],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'decimals',
    stateMutability: 'view',
    inputs: [],
    outputs: [{ name: '', type: 'uint8' }],
  },
] as const;
