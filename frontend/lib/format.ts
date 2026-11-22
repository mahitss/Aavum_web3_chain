export function parseTusd(input: string): bigint {
  const trimmed = input.trim();
  if (!/^\d+(\.\d{0,6})?$/.test(trimmed)) {
    throw new Error('Enter a tUSD8 amount with at most 6 decimals');
  }
  const [whole, frac = ''] = trimmed.split('.');
  return BigInt(whole) * 1_000_000n + BigInt((frac + '000000').slice(0, 6));
}

export function formatTusd(units: bigint): string {
  const negative = units < 0n;
  const value = negative ? -units : units;
  const whole = value / 1_000_000n;
  const frac = (value % 1_000_000n).toString().padStart(6, '0').replace(/0+$/, '');
  const text = frac.length > 0 ? `${whole}.${frac}` : whole.toString();
  return negative ? `-${text}` : text;
}

export function errorText(error: unknown): string {
  if (error instanceof Error) {
    return error.message.split('\n')[0];
  }
  return 'request failed';
}
