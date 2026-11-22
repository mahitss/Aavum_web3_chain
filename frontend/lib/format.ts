import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function tusd(units: bigint | number | string | undefined) {
  if (units === undefined) return '—';
  const value = typeof units === 'bigint' ? units : BigInt(units || 0);
  const whole = value / 1_000_000n;
  const frac = (value % 1_000_000n).toString().padStart(6, '0').replace(/0+$/, '');
  return frac ? `${whole}.${frac}` : whole.toString();
}

export function shortHash(hash: string) {
  if (!hash || hash.length < 12) return hash || '—';
  return `${hash.slice(0, 8)}…${hash.slice(-4)}`;
}
