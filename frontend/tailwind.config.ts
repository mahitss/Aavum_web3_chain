import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./app/**/*.{ts,tsx}', './components/**/*.{ts,tsx}', './lib/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['var(--font-sans)', 'sans-serif'],
        mono: ['var(--font-mono)', 'ui-monospace', 'monospace'],
      },
      colors: {
        ink: '#090b10',
        panel: '#10151c',
        line: '#243140',
        signal: '#3ee0c5',
        worker: '#e7b549',
        orch: '#7aa2ff',
        danger: '#ff3b4e',
      },
    },
  },
  plugins: [],
};

export default config;
