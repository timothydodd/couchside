# Couchside style guide

The palette comes from the C logo (`branding/logo-c-original.png`): a cyan rim
around an electric-blue body that turns azure and then seafoam at the arm
tips, on a deep navy tile. The app itself sits on neutral dark grays, so the
logo's cool colours carry the brand and navy stays inside the logo tile. The
tokens live in `web/src/index.css`; components use them through Tailwind
utilities (`bg-surface`, `text-content`, `text-accent`…). Never hard-code a
colour: both themes must keep working.

## Logo colours (sampled)

| Name          | Hex       | Where it is in the logo           |
| ------------- | --------- | --------------------------------- |
| Navy          | `#0C1930` | the tile behind the C             |
| Cyan          | `#17FBFE` | the rim and the top-left glow     |
| Electric blue | `#016FF0` | the body of the C                 |
| Deep blue     | `#003FB1` | the shaded inner curve            |
| Azure         | `#06B6F3` | the arms, between blue and green  |
| Seafoam       | `#5AF6B9` | the arm tips                      |

## Tokens

| Token              | Dark      | Light     | Use                                                    |
| ------------------ | --------- | --------- | ------------------------------------------------------ |
| `--bg-page`        | `#111315` | `#F3F4F6` | the app background                                     |
| `--bg-surface`     | `#181B1F` | `#FFFFFF` | cards, sidebar, panels                                 |
| `--bg-raised`      | `#22262B` | `#F8F9FA` | things on a surface: guide cells, menus                |
| `--bg-muted`       | white 5%  | gray 5%   | hover rows, quiet fills                                |
| `--text-primary`   | `#ECEFF3` | `#15181C` | titles, body                                           |
| `--text-secondary` | `#B3BAC4` | `#3F4650` | supporting text                                        |
| `--text-muted`     | `#8C949F` | `#5E6672` | captions, metadata                                     |
| `--border`         | `#343A42` | `#D5D9DF` | inputs, outlined buttons                               |
| `--border-light`   | `#262A30` | `#E6E8EC` | card edges, dividers                                   |
| `--accent`         | `#5B93FF` | `#1A56E8` | primary buttons, active nav, links, progress           |
| `--accent-hover`   | `#7EA9FF` | `#1446C4` | hover on accent fills                                  |
| `--text-on-accent` | `#07122A` | `#FFFFFF` | text on accent (and status) fills                      |
| `--brand-cyan`     | `#1FEEFE` | `#08788F` | cyan: small highlights, avatar colour                  |
| `--brand-seafoam`  | `#5AF6B9` | `#0B7F5C` | seafoam: the guide's "now" line, placeholder glow      |
| `--brand-gradient` | sweep     | sweep     | cyan → blue → azure → seafoam, via `.brand-text` only  |
| `--secondary`      | `#A873FF` | `#6A24E0` | violet: avatar colour only (from the old logo)         |
| `--brand-pink`     | `#FF4FE6` | `#B8169F` | pink: avatar colour only (from the old logo)           |
| `--critical`       | `#FF5C74` | `#CC2B45` | errors, destructive actions, "recording"               |
| `--warning`        | `#FFB547` | `#A35F00` | unmatched, low quality, duplicates                     |
| `--good`           | `#3DDC97` | `#107553` | watched, healthy, done                                 |
| `--info`           | `#45C6FF` | `#0B6FB8` | neutral notices, in progress                           |

Violet and pink stay because profiles have them saved as avatar colours; they
aren't part of the brand any more and shouldn't be used anywhere else.

## Rules

- **Contrast.** Every text colour above passes WCAG AA (4.5:1) on the page,
  surface and raised backgrounds in both themes (violet and pink are only ever
  fills). Re-check if you add a pairing.
- **Dark buttons carry dark text.** The dark accent is bright (`#5B93FF`), so
  white on it fails contrast; `--text-on-accent` is a near-black navy. Always
  pair accent fills with `text-on-accent`, never `text-white`.
- **One accent.** Blue is the only interactive colour. Cyan and seafoam are
  brand colours for identity and small highlights, not for buttons or links.
- **Seafoam isn't "good".** Seafoam and the green status colour are neighbours.
  Keep seafoam to brand moments (the gradient, the guide's now line, artwork
  placeholders) and never use it to mean done, watched or healthy.
- **Navy is the logo's.** The UI is neutral gray; navy appears only in the logo
  tile and the app icons, never as a surface.
- **The gradient is a signature, not a background.** Use `.brand-text` for
  short brand labels ("media" in the sidebar, "Just added" on Home). Not for
  body copy, buttons or large fills.
- **Status colours mean state.** Critical, warning, good and info are never
  used decoratively or as chart series.
- **The player is always dark** (`data-theme="dark"` on its root), whatever
  the app theme.
- **New reusable styles** go in `index.css` under `@layer components`, named
  like the existing ones (`.poster`, `.still`, `.chip`, `.badge`, `.card`).

## Artwork

- `branding/logo-c-original.png` is the source: the C on its navy tile.
- `python3 branding/make-icons.py` writes the web icons (`web/public/icons`,
  `favicon.ico`, `apple-touch-icon.png`), and `python3 branding/make-transparent.py`
  cuts the C out to `branding/logo-c-transparent.png`.
- The Roku app's channel icons are the C on the logo navy; its splash is the C
  on the dark page gray (see `../couchside-roku`).
