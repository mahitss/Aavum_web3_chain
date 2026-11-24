# Design system

The command center is a dark operations surface, not a marketing page.

| Token | Value | Use |
|---|---|---|
| canvas | `#05070a` | Page background |
| card | `#0c1017` | Panels |
| emerald | `#10b981` | Orchestrator node and primary action |
| cyan | `#06b6d4` | Worker node and hashes |
| amber | `#f59e0b` | Escrow node and approve |
| panic | `#ef4444` | Kill switch and errors |
| line | `#243140` | Borders |

Type is the system stack named in `frontend/app/globals.css`: Outfit if the machine has it, otherwise Segoe UI or system-ui. Mono is IBM Plex Mono if installed, otherwise Cascadia Mono or ui-monospace. The production image does not download Google Fonts at build time.

Spacing is Tailwind's default scale. Panels use `rounded-2xl` or `rounded-3xl`. The mesh is full width above a two-column split: controls on the left, terminals on the right. On a narrow screen the columns stack.

Do not add a second accent color for decoration. Status text is `OPERATIONAL`, `INACTIVE`, or `SESSION_REVOKED`.
