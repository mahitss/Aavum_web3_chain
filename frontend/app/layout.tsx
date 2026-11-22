import type { Metadata } from 'next';
import { IBM_Plex_Mono, Outfit } from 'next/font/google';
import '@rainbow-me/rainbowkit/styles.css';
import './globals.css';
import { Providers } from './providers';

const outfit = Outfit({ subsets: ['latin'], variable: '--font-sans' });
const mono = IBM_Plex_Mono({
  subsets: ['latin'],
  weight: ['400', '500'],
  variable: '--font-mono',
});

export const metadata: Metadata = {
  title: 'Nexus402 Command Center',
  description: 'Session delegation, HTTP 402 settlement, and emergency revoke on QDay Aevum.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={`${outfit.variable} ${mono.variable} font-sans antialiased`}>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
