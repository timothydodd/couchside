# Couchside style guide

**Seafoam on slate.** One dark theme: navy-tinted charcoals that sit well with
the logo's navy tile, seafoam (the tips of the C) as the colour you act on,
sky blue for state, coral for warmth. There is no light theme. The tokens
live in `web/src/index.css`; components use them through Tailwind utilities
(`bg-surface`, `text-content`, `text-accent`…). Don't hard-code colours.

## Logo

`branding/logo-c-original.png` is the C on its navy tile (`#0D1A31`): a cyan
rim around an electric-blue body that turns azure and then seafoam at the arm
tips. The tile stays in the app icons and the Roku channel art; the UI around
it is slate.

## Tokens

| Token              | Value     | Use                                                      |
| ------------------ | --------- | -------------------------------------------------------- |
| `--bg-page`        | `#0D1418` | the app background (near-black navy)                     |
| `--bg-surface`     | `#151F24` | sidebar, cards, panels (dark charcoal)                   |
| `--bg-raised`      | `#1D292E` | things on a surface: guide cells, menus, chips (slate)   |
| `--bg-muted`       | white 5%  | hover rows, quiet fills                                  |
| `--text-primary`   | `#EDF4F2` | titles, body (cool white)                                |
| `--text-secondary` | `#9DAEAA` | supporting text (gray-green)                             |
| `--text-muted`     | `#869894` | captions, metadata                                       |
| `--text-disabled`  | `#667570` | disabled controls only (below AA on purpose)             |
| `--border`         | `#55696F` | inputs, outlined buttons (3:1 against the field)         |
| `--border-light`   | `#304047` | card edges, dividers (muted slate)                       |
| `--accent`         | `#63D6BE` | seafoam: buttons, links, focus, progress, active nav     |
| `--accent-hover`   | `#7DE3CE` | hover on accent fills                                    |
| `--text-on-accent` | `#0D1418` | text on seafoam (and other bright) fills                 |
| `--brand-sky`      | `#62A9D1` | sky blue: the second brand colour                        |
| `--brand-coral`    | `#F08070` | coral: the warm accent                                   |
| `--brand-gradient` | sweep     | seafoam → sky, via `.brand-text` only                    |
| `--good`           | `#62A9D1` | watched, done, healthy (sky)                             |
| `--info`           | `#A9D2EA` | in progress, neutral notices (light sky)                 |
| `--warning`        | `#E8B65A` | unmatched, low quality, duplicates (amber)               |
| `--critical`       | `#F08070` | errors, destructive actions, recording, favourites (coral) |
| `--brand-cyan`, `--secondary` (violet), `--brand-pink` | | avatar colours only |

## Contrast

| Pair                                  | Ratio      |
| ------------------------------------- | ---------- |
| Primary text on page / surface / raised | 16.7 / 15.0 / 13.4 |
| Secondary text, worst case (raised)   | 6.4        |
| Muted text, worst case (raised)       | 4.9        |
| Seafoam on raised                     | 8.4        |
| Sky / coral on raised                 | 5.8 / 5.7  |
| Navy text on seafoam                  | 10.5       |
| Input border against the field        | 3.2        |

## Rules

- **One accent.** Seafoam is the only interactive colour. Sky, coral and amber
  mean something; they're never used for buttons or links.
- **No green.** Seafoam is too close to a green to share the screen with one,
  so "watched" and "done" are sky blue.
- **Bright fills carry dark text.** Seafoam, sky, coral and amber fills take
  `text-on-accent` (`#0D1418`), never white.
- **Coral means attention:** recording, errors, deletes, and the favourite
  heart. Keep it rare so it stays noticeable.
- **The gradient is a signature, not a background.** `.brand-text` for short
  brand labels only.
- **The player is always on black** (`--player-bg`), whatever is behind it.
- **New reusable styles** go in `index.css` under `@layer components`, named
  like the existing ones (`.poster`, `.still`, `.chip`, `.badge`, `.card`).

## Artwork

- `python3 branding/make-icons.py` writes the web icons from the logo, and
  `python3 branding/make-transparent.py` cuts the C out to
  `branding/logo-c-transparent.png`.
- The Roku app's artwork comes from `../couchside-roku/scripts/make-art.py`:
  channel icons are the C on the logo's navy; the splash is the C on the page
  colour; 9-patches and badges are recoloured to these tokens.
