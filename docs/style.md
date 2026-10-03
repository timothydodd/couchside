# Couchside style guide

**Seafoam as the primary.** One dark theme: neutral dark greys, bright
seafoam (the tips of the C) as the colour you act on, the logo's blue for
state, a warm red for attention. There is no light theme. The tokens live in
`web/src/index.css`; components use them through Tailwind utilities
(`bg-surface`, `text-content`, `text-accent`…). Don't hard-code colours.

## Logo

`branding/logo-c-original.png` is the C on its navy tile (`#0D1A31`): a cyan
rim around an electric-blue body that turns azure and then seafoam at the arm
tips. The tile stays in the app icons and the Roku channel art; the UI around
it is grey.

## Tokens

| Token              | Value     | Use                                                      |
| ------------------ | --------- | -------------------------------------------------------- |
| `--bg-page`        | `#111315` | the app background                                       |
| `--bg-surface`     | `#181B1F` | sidebar, cards, panels                                   |
| `--bg-raised`      | `#22262B` | things on a surface: guide cells, menus, chips           |
| `--bg-muted`       | white 5%  | hover rows, quiet fills                                  |
| `--text-primary`   | `#ECEFF3` | titles, body                                             |
| `--text-secondary` | `#B3BAC4` | supporting text                                          |
| `--text-muted`     | `#8C949F` | captions, metadata                                       |
| `--text-disabled`  | `#666D77` | disabled controls only (below AA on purpose)             |
| `--border`         | `#626A75` | inputs, outlined buttons (3:1 against the field)         |
| `--border-light`   | `#262A30` | card edges, dividers                                     |
| `--accent`         | `#5AF6B9` | seafoam: buttons, links, focus, progress, active nav     |
| `--accent-hover`   | `#8CF9CF` | hover on accent fills                                    |
| `--text-on-accent` | `#04261A` | deep green text on seafoam (and other bright) fills      |
| `--brand-sky`      | `#5B93FF` | the logo's blue: the second brand colour                 |
| `--brand-coral`    | `#FF5C74` | warm red: the attention colour                           |
| `--brand-gradient` | sweep     | cyan → blue → sky → seafoam, via `.brand-text` only      |
| `--good`           | `#5B93FF` | watched, done, healthy (blue)                            |
| `--info`           | `#38D6F5` | in progress, neutral notices (sky)                       |
| `--warning`        | `#FFB547` | unmatched, low quality, duplicates (amber)               |
| `--critical`       | `#FF5C74` | errors, destructive actions, recording, favourites (red) |
| `--brand-cyan`, `--secondary` (violet), `--brand-pink` | | avatar colours only |

## Contrast

| Pair                                    | Ratio              |
| --------------------------------------- | ------------------ |
| Primary text on page / surface / raised | 16.1 / 15.0 / 13.2 |
| Secondary text, worst case (raised)     | 7.8                |
| Muted text, worst case (raised)         | 5.0                |
| Seafoam on raised                       | 11.1               |
| Blue / red on raised                    | 5.1 / 5.1          |
| Deep green text on seafoam              | 11.8               |
| Dark text on blue / red fills           | 5.4 / 5.4          |
| Input border against the field          | 3.4                |

## Rules

- **One accent.** Seafoam is the only interactive colour. Blue, red and amber
  mean something; they're never used for buttons or links.
- **No green.** Seafoam is too close to a green to share the screen with one,
  so "watched" and "done" are the logo's blue.
- **Bright fills carry dark text.** Seafoam, blue, red and amber fills take
  `text-on-accent` (`#04261A`), never white.
- **Red means attention:** recording, errors, deletes, and the favourite
  heart. Keep it rare so it stays noticeable.
- **The gradient is a signature, not a background.** `.brand-text` for short
  brand labels only.
- **The player is always on black** (`--player-bg`), whatever is behind it.
- **Charts** use state colours, not the accent: the whole machine in muted grey
  (`--text-muted`, with a faint fill), Couchside in blue (`--good`), its encoders
  in sky (`--info`). Grid lines are `--border-light`.
- **New reusable styles** go in `index.css` under `@layer components`, named
  like the existing ones (`.poster`, `.still`, `.chip`, `.badge`, `.card`).

## Artwork

- `python3 branding/make-icons.py` writes the web icons from the logo, and
  `python3 branding/make-transparent.py` cuts the C out to
  `branding/logo-c-transparent.png`.
- The Roku app's artwork comes from `../couchside-roku/scripts/make-art.py`:
  channel icons are the C on the logo's navy; the splash is the C on the page
  colour; 9-patches and badges are recoloured to these tokens.
