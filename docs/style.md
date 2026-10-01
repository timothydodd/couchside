# Couchside style guide

The palette comes from the C logo (`branding/logo-c-original.png`): a deep navy
ground with a cyan → electric blue → violet → magenta sweep. The tokens live in
`web/src/index.css`; components use them through Tailwind utilities
(`bg-surface`, `text-content`, `text-accent`…). Never hard-code a colour: both
themes must keep working.

## Logo colours (sampled)

| Name          | Hex       | Where it is in the logo        |
| ------------- | --------- | ------------------------------ |
| Navy          | `#0E1C37` | the background                 |
| Cyan          | `#1FEEFE` | the top-left rim and glow      |
| Electric blue | `#076BFD` | the body of the C              |
| Royal blue    | `#103CE0` | the shaded inner curve         |
| Violet        | `#882FFC` | where the arms turn            |
| Magenta       | `#FA27E8` | the arm tips                   |

## Tokens

| Token              | Dark      | Light     | Use                                                |
| ------------------ | --------- | --------- | -------------------------------------------------- |
| `--bg-page`        | `#0A1428` | `#F2F5FC` | the app background                                 |
| `--bg-surface`     | `#0F1D39` | `#FFFFFF` | cards, sidebar, panels (dark = the logo's navy)    |
| `--bg-raised`      | `#172A4F` | `#F7F9FE` | things on a surface: guide cells, menus            |
| `--bg-muted`       | blue 6%   | navy 5%   | hover rows, quiet fills                            |
| `--text-primary`   | `#EEF2FB` | `#0E1C37` | titles, body                                       |
| `--text-secondary` | `#B4BFD9` | `#3B4A6B` | supporting text                                    |
| `--text-muted`     | `#8592B3` | `#5D6B8C` | captions, metadata                                 |
| `--border`         | `#2A3C63` | `#CFD8EA` | inputs, outlined buttons                           |
| `--border-light`   | `#1D2D4F` | `#E4E9F5` | card edges, dividers                               |
| `--accent`         | `#5B93FF` | `#1A56E8` | primary buttons, active nav, links, progress       |
| `--accent-hover`   | `#7EA9FF` | `#1446C4` | hover on accent fills                              |
| `--text-on-accent` | `#07122A` | `#FFFFFF` | text on accent (and status) fills                  |
| `--secondary`      | `#A873FF` | `#6A24E0` | violet: a second highlight, avatar colour          |
| `--brand-pink`     | `#FF4FE6` | `#B8169F` | magenta: the guide's "now" line, avatar colour     |
| `--brand-cyan`     | `#1FEEFE` | `#08788F` | cyan: avatar colour, small highlights              |
| `--brand-gradient` | sweep     | sweep     | the logo's colours, via `.brand-text` only         |
| `--critical`       | `#FF5C74` | `#CC2B45` | errors, destructive actions, "recording"           |
| `--warning`        | `#FFB547` | `#A35F00` | unmatched, low quality, duplicates                 |
| `--good`           | `#3DDC97` | `#12805A` | watched, healthy, done                             |
| `--info`           | `#45C6FF` | `#0B6FB8` | neutral notices, in progress                       |

## Rules

- **Contrast.** Every text colour above passes WCAG AA (4.5:1) on the page,
  surface and (except violet, which is decorative) raised backgrounds, in both
  themes. Re-check if you add a pairing.
- **Dark buttons carry navy text.** No single blue is light enough to read as
  text on navy and dark enough to hold white text, so the dark accent is
  bright (`#5B93FF`) and `--text-on-accent` is navy. Always pair accent fills
  with `text-on-accent`, never `text-white`.
- **One accent.** Blue is the only interactive colour. Violet, magenta and
  cyan are brand colours for identity and small highlights, not for buttons or
  links.
- **The gradient is a signature, not a background.** Use `.brand-text` for
  short brand labels ("media" in the sidebar, "Just added" on Home). Not for
  body copy, buttons or large fills.
- **Status colours mean state.** Critical, warning, good and info are never
  used decoratively or as chart series.
- **The player is always dark** (`data-theme="dark"` on its root), whatever
  the app theme.
- **New reusable styles** go in `index.css` under `@layer components`, named
  like the existing ones (`.poster`, `.still`, `.chip`, `.badge`, `.card`).
