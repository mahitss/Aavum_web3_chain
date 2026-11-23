import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./app/**/*.{ts,tsx}', './components/**/*.{ts,tsx}', './lib/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['var(--font-sans)', 'sans-serif'],
        mono: ['var(--font-mono)', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
      colors: {
        canvas: '#05070a',
        card: '#0c1017',
        emerald: '#10b981',
        cyan: '#06b6d4',
        amber: '#f59e0b',
        panic: '#ef4444',
        line: '#243140',
      },
    },
  },
  plugins: [],
};

export default config;
