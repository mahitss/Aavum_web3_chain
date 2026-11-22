import { isAddress } from 'viem';

export const TUSD8_ADDRESS = '0x456569838f1d43009A91ebA4263147Afd7233483' as const;

const configured = process.env.NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS ?? '';
export const NEXUS_ESCROW_ADDRESS = isAddress(configured) ? configured : undefined;

export const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8081';

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
    ],
  },
  {
    type: 'function',
    name: 'nonces',
    stateMutability: 'view',
    inputs: [{ name: 'principal', type: 'address' }],
    outputs: [{ name: '', type: 'uint256' }],
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
] as const;
