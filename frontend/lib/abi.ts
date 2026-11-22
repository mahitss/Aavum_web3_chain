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
      { name: 'nonce', type: 'uint256' },
      { name: 'signature', type: 'bytes' },
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
    name: 'nonces',
    stateMutability: 'view',
    inputs: [{ name: 'principal', type: 'address' }],
    outputs: [{ name: '', type: 'uint256' }],
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
      {
        name: '',
        type: 'tuple',
        components: [
          { name: 'agentKey', type: 'address' },
          { name: 'hourlyLimit', type: 'uint256' },
          { name: 'perTaskLimit', type: 'uint256' },
          { name: 'expiresAt', type: 'uint256' },
          { name: 'currentWindow', type: 'uint256' },
          { name: 'spentInWindow', type: 'uint256' },
          { name: 'isActive', type: 'bool' },
        ],
      },
    ],
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
    ],
  },
  {
    type: 'event',
    name: 'TaskEscrowLocked',
    inputs: [
      { name: 'taskId', type: 'bytes32', indexed: true },
      { name: 'principal', type: 'address', indexed: true },
      { name: 'agentKey', type: 'address', indexed: true },
      { name: 'worker', type: 'address', indexed: false },
      { name: 'amount', type: 'uint256', indexed: false },
      { name: 'deadline', type: 'uint256', indexed: false },
      { name: 'taskHash', type: 'bytes32', indexed: false },
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
  { type: 'error', name: 'InvalidSignature', inputs: [] },
  { type: 'error', name: 'SessionExpired', inputs: [] },
  { type: 'error', name: 'SessionInactive', inputs: [] },
  { type: 'error', name: 'ExceedsPerTaskLimit', inputs: [] },
  { type: 'error', name: 'ExceedsHourlyLimit', inputs: [] },
  { type: 'error', name: 'InvalidWorkerAddress', inputs: [] },
  { type: 'error', name: 'TaskNotFound', inputs: [] },
  { type: 'error', name: 'TaskNotPending', inputs: [] },
  { type: 'error', name: 'DeadlineNotPassed', inputs: [] },
  { type: 'error', name: 'InvalidProofSignature', inputs: [] },
  { type: 'error', name: 'ZeroAmount', inputs: [] },
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

export const sessionTypes = {
  SessionAuthorization: [
    { name: 'principal', type: 'address' },
    { name: 'agentKey', type: 'address' },
    { name: 'hourlyLimit', type: 'uint256' },
    { name: 'perTaskLimit', type: 'uint256' },
    { name: 'expiresAt', type: 'uint256' },
    { name: 'nonce', type: 'uint256' },
  ],
} as const;
