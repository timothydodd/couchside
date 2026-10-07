# UI polish stories

A pass over the web app (6 October 2026, checked against `main` at
`fd839fd`), planned as stories `P1` to `P16`. The first five are the title-page rework:
episodes as a grid with their own page, a cast strip that can be scrolled,
and one place to edit a movie or show. The rest came out of an audit of
every page.

Line numbers will drift; the component and function names are the stable
reference. Work goes on the `ui-polish` branch, one commit per story titled
`P6: …`, and the story is marked Done here in the same commit.

Suggested order: **P6** first (the dialogs everything else confirms with),
**P4**, **P1 → P3**, **P5** (it reuses P1's card and P2's page), then
**P7 → P9** (shared primitives), then **P10 → P16** as filler.

| ID  | Story                                                   | Kind     | Size | Done |
| --- | ------------------------------------------------------- | -------- | ---- | ---- |
| P1  | Episodes as a grid, list on phones                      | Feature  | S    | Done |
| P2  | Episode page: plot, still, guest stars, crew            | Feature  | M    | Done |
| P3  | Picture plays, everything else opens the page           | Feature  | S    | Done |
| P4  | Rows that scroll: cast, Home, search                    | Fix      | S    | Done |
| P5  | Edit a title from its page; Manage is library settings  | Feature  | M    | Done |
| P6  | Couchside's own confirm and prompt dialogs              | Fix      | S    | Done |
| P7  | Shared badge, empty, loading and error blocks           | Cleanup  | S    | Done |
| P8  | Back buttons and page headers behave alike              | Fix      | S    | Done |
| P9  | Phone layout: rows that crowd or run off                | Fix      | S    | Done |
| P10 | Home: the hero plays, and says why it's empty           | Fix      | S    | Done |
| P11 | Guide: time shifts show a loading state                 | Fix      | S    |      |
| P12 | Copy: one name for things, admin-only advice to admins  | Fix      | S    |      |
| P13 | Status is summarised once                               | Cleanup  | S    |      |
| P14 | Keyboard: tabs, radios, menus                           | A11y     | S    |      |
| P15 | Search: see more, locked channels explained             | Fix      | S    |      |
| P16 | Small smells                                            | Cleanup  | S    |      |

**Rules for every story**
- One story per commit on `ui-polish`. Don't fold neighbouring cleanups in.
- Styling: reuse `index.css` classes (`.still`, `.poster`, `.badge`,
  `.chip`, `.row-title`, `.side-panel`, `.panel-section`) and the tokens in
  `docs/style.md`. New reusable styles go under `@layer components`.
- Verify with `npx tsc --noEmit && npx vite build` in a scratch copy of
  `web/` (see `CLAUDE.md`), and `go vet ./... && go test ./...` in `server/`
  for P2 and P5.

---

# The title page

## P1 · Episodes as a grid, list on phones · Done

**Why.** `Seasons` listed episodes as rows with a 176px still and one line
of text, so a 22-episode season was a long scroll and most of the width
was empty on a desktop.

**Done.** `components/EpisodeCard.tsx` is the one tile for episodes and
extras: the still (or placeholder) with the play button on hover, the
progress strip, watched and problem as `.art-badge` overlays, the admin
"⋯" top-left on hover, and the caption under it (`E4 · Title`, then date,
runtime and rating). `Seasons` and `Extras` lay the cards out in an
auto-fill grid (220px minimum) from `md` up and as the old list
(`layout="row"`) on phones, through `TileList`. The season tabs fade at
the end that scrolls and bring the open season's tab into view. The old
`StillRow` and `EpisodeItem` are gone. The caption becomes a link in P3,
once P2 gives it a page.

## P2 · Episode page: plot, still, guest stars, crew · Done

**Why.** Clicking an episode played it; there was nowhere to read what it's
about or who's in it. The `episodes` table held only title, date, rating
and IMDb id, and `tmdbSeason` dropped TMDB's `overview`, `still_path`,
`runtime`, `guest_stars` and `crew`.

**Done, server.** Migration 0034 adds `episodes.plot`, `runtime_min`,
`still_url`, `tmdb_id` and an `episode_credits` table (same shape as
`item_credits`, shared `people`, both pruned together). `metadata.Episode`
carries them; TMDB's season decoder fills them (guest stars capped like the
cast, crew filtered by `crewJobs`); OMDb leaves them empty and doesn't wipe
TMDB's credits. `GET /api/episodes/{id}` returns the episode, its show,
every copy, guest stars, crew and the episodes either side; `GET
/api/people/{id}` adds `episodes`. The artwork job downloads each episode's
TMDB still over the frame grab (`episodeStill`, once per link via
`still.src`), and the still job keeps a provider still when a file is
re-scanned. Existing libraries pick it up on the next match (Libraries →
Re-match).

**Done, web.** `/episode/{id}` (`pages/EpisodePage.tsx`): the still as the
hero, the show's name linking back to `/item/{id}?season=N` (which
`Seasons` opens on), `S2 · E4 · Title`, date, runtime, quality and rating,
Play or Resume, Mark watched and the admin "⋯", the synopsis, Director and
Writer, "Guest stars", Previous and Next, and the copies table (now
`components/item/FilesCard.tsx`) when there's more than one. A person's
page lists the episodes they guest in.

## P3 · Picture plays, everything else opens the page · Done

**Why.** The whole episode row was one link to the player. With P2 there
are two destinations, and `ContinueCard` already drew the line: the
picture resumes, the title opens the page.

**Done.** In `EpisodeCard` the still is a link to the player (with the
hover play button and a "Play S2 E4 Title" label) and the caption a
`title-link` to `/episode/{id}`; nothing in between is clickable. A file
with a problem has no play link, so its picture opens the page too, where
the problem is explained. Posters keep opening the title page; only 16:9
tiles play from the picture, which `docs/style.md` now says under "What a
click does". Both links are in the tab order.

## P4 · Rows that scroll: cast, Home, search · Done

**Why.** `Row` (`components/Rows.tsx`) hid the scrollbar and offered nothing
else, so with a mouse the cast strip, Continue Watching, Recently Added and
the search result rows could only be scrolled with shift-wheel, and a cast
of 20 ran off the right edge with no sign there was more.

**Done.** `useScrollEdges` (`lib/scroll.ts`) watches a scroller and sets
`data-fade`; `.row-scroll` masks the end that can still scroll, and
`scroll-padding-inline` keeps a focused card clear of the fade. `Row`
shows paging arrows in its heading when there's more than fits, disabled at
each end and hidden for coarse pointers (`.row-nav`). The Settings phone
tab bar gets the fade. The season tabs follow in P1.

## P5 · Edit a title from its page; Manage is library settings · Done

**Why.** The title page's "⋯" had Edit details, jobs and delete, but the
poster and backdrop, searching for the right match, file roles and deleting
one copy lived only in a panel behind `/libraries/{id}`, reached by
"Manage in library" and then finding the title again in a table.

**Done.** `GET /api/items/{id}/manage` returns one title's Manage row and
its library. The title page (and the episode page) has an admin **Edit**
button beside Mark watched, opening `ItemPanel` as a drawer over the page
with Details (`components/item/DetailsEditor.tsx`, the old `EditDetails`
modal as an inline section), Match (`MetadataSearch`), Artwork
(`ArtworkEditor`) and Files (`FileList`: roles, extra copies, delete). On
an episode's page the panel lists that episode's copies and links to the
show for the rest. Jobs and deleting the whole title stay in the "⋯",
whose "Manage in library" is now "Library settings". "Fix match" on the
title page opens the panel on Match; `/item/{id}?edit=1` opens it on
Details (the library table, and the Movies and TV grids' "Edit…", link
there). `/libraries/{id}` is now the library's settings page: a card with
its name, folder and options (Edit, through the shared
`components/LibraryForm.tsx`), Scan, Re-match, Optimize all and Remove,
and the titles table under it with each title linking to its page. The
Libraries list's split button says Settings and its menu lost Edit.

---

# Everywhere else

## P6 · Couchside's own confirm and prompt dialogs · Done

**Why.** Actions were wrapped in `attempt()` for failures but still asked
with the browser's `confirm()`, `prompt()` and `alert()` in sixteen places,
which look foreign in the dark theme and can't be styled or focus-trapped.

**Done.** `lib/ask.ts` has `confirmDialog({title, body, action, danger})`
and `promptDialog({title, body, value, placeholder})`, answered by
`components/Dialogs.tsx` (mounted in `main.tsx` beside `Notices`): a
`role="alertdialog"` card from the shared `components/Modal.tsx`, Enter
answers (Cancel takes focus first for a destructive question), Escape
cancels. The two `alert()`s are `notify(…, "info")`. `EditDetails`,
`MergeTitles` and `DeleteSelected` use `Modal` instead of their own
overlay. `useDialog` keeps a stack, so a confirm opened over a panel or
dialog takes Escape and Tab until it closes. `ItemPanel`'s own `Note`
block stays until P5 removes the panel's job menu.

## P7 · Shared badge, empty, loading and error blocks · Done

**Done.** Every hand-rolled `tint-* rounded px-1.5 text-[11px]
font-semibold` label is `.badge` (Sidebar, MobileNav, AccountManager,
SessionList, LibrariesPage, ProgramDialog, Recordings, LiveTvSettings);
the guide's 9px HD and NEW marks and the player's REC are sized for their
own chrome and stay. `EmptyState` takes an `action`, icons are 36 across
pages, "Nothing matches those filters" has Reset filters, and the Search
and Guide empties have icons. `Loading` (`ui.tsx`) replaces the three
spinner wrappers; Libraries, Activity and Backups now show it instead of
nothing or an empty table, and Two-step sign-in shows its load error.
`GoodNote` joins `ErrorNote`/`WarningNote` and is used by the Edit panel,
Link a TV, Single sign-on (with `ErrorNote` for failures) and the
recovery codes. Backups and "Didn't record" tables have headers; the
Backups delete chip has the critical hover; Two-step's Cancel reports a
failure.

## P8 · Back buttons and page headers behave alike · Done

**Done.** `BackButton` (`ui.tsx`: `fallback`, `label`, `overlay`) is the
one back control: the title, episode and person pages float it over their
hero, the library settings page and sign-in use the plain one. A person's
page has a hero now (their photo blown up and blurred, as a title with no
backdrop gets its poster), with phone sizes for the photo and name. The
Movies and TV header's top padding matches `PageHeader`. Live TV's tabs
carry `aria-current`, and `.navtab` has a transparent base border so the
active tab no longer grows by 2px.

## P9 · Phone layout: rows that crowd or run off · Done

**Done.** Live TV's guide error is a `WarningNote` under the tabs, not a
span in the scrolling tab row. The "Recording now" card wraps its buttons
onto their own row below `sm`. A library's row on the Libraries page puts
its counts and split button on a second row below `sm` (`sm:contents`
restores the single line). `StatTile` truncates its sub-line, so the
running job's label can't widen the Activity tiles. The Settings phone
tab bar's fade came with P4.

## P10 · Home: the hero plays, and says why it's empty · Done

**Done.** The hero's button is Play (or "Resume 12:30"), going straight to
the file `playTarget` (`lib/items.ts`, the title page's rule, shared) picks
from the item the hero already fetched for its plot. The hero is picked
once per visit and only re-picked if it leaves the recent rows, so the
15s poll can't swap it mid-read. With libraries but no titles, Home says
"Scanning your libraries…" while jobs run, or "No titles yet" with a Scan
now button for admins. The Unmatched tile links admins to Settings →
Metadata instead of naming an environment variable.

## P11 · Guide: time shifts show a loading state

**Scope** (`components/livetv/Guide.tsx`)
- `:44`: shifting the window changes the header at once but the old
  programs stay (`useApi` with `keep`) until the fetch returns, and the
  spinner only shows on first load (`:49`). Dim the grid (`opacity-50 pointer-events-none`) while
  `loading`, or keep the old window's header until the new data lands.
- `:69`: "Earlier" is never disabled; disable it at the guide's start.
- `:68-83`: the time controls are a separate row pulled up with `-mt-3`,
  while `FilterBar` has a `children` slot for them (`FilterBar.tsx:21,35`).
  Use the slot.

## P12 · Copy: one name for things, admin-only advice to admins

**Scope**
- "TV Shows" (`LibraryPage.tsx:155`, `Sidebar.tsx:17`, the library form's
  default name `LibrariesPage.tsx:147,169`), "TV shows" (`HomePage.tsx:33,60`,
  `SearchPage.tsx:61`, `PersonPage.tsx:64`, `LibrariesPage.tsx:37,184`),
  "TV" (`MobileNav.tsx:51`, `LibrariesPage.tsx:83`, `LibraryPage.tsx:275`).
  Use "TV shows" in headings and rows, "TV" only where space is short
  (bottom tabs, badges).
- `LiveTvPage.tsx:23`: the empty state links to `/settings` (Live TV is
  `/settings/livetv`, admin-only) and shows restart and env-var
  instructions to everyone. `LibraryPage.tsx:274` sends non-admins to
  Libraries. Copy `HomePage.tsx:29`'s admin split.
- `ProgramDialog.tsx:47` names Plex in the tuner-conflict message; say
  "another recorder".
- `ActivityPage.tsx:57`: "Clear finished" is enabled only for `done` jobs;
  failed jobs are finished too.
- `StatusBar.tsx:20` says "N jobs queued", `MobileNav.tsx:141` "N jobs
  running" (never singular) for the same number; `Sidebar.tsx:49` shows
  failed and hides active, `MobileNav.tsx:127` the reverse. One wording and order (P13
  gives them one source).
- `Sidebar.tsx:94`: the "Manage" section heading shows for users who only
  have Settings in it. `MobileNav.tsx:57` highlights "More" on the search
  page although search is in the top bar (`:60`).

## P13 · Status is summarised once

**Scope**
- The REC badge is built in `Sidebar.tsx:43`, `MobileNav.tsx:30` and
  `StatusBar.tsx:42`; the job summary in `Sidebar.tsx:48`,
  `MobileNav.tsx:94,127` and `StatusBar.tsx:10,20` (`fmtVersion` is
  shared now, placed in the same three spots). A `useStatusSummary()` in `stores/status.ts` returning `{recording,
  jobs: {active, queued, failed}, label, version}` and a `RecBadge`
  component.
- Nav items are listed twice (`Sidebar.tsx:14-26`, `MobileNav.tsx:45-56`
  plus the hard-coded More sheet rows at `:121-134`). One `NAV` table in
  `stores/router.ts` with `phone: "tab" | "more"`.
- `SearchPage.tsx:118` `Section` and `PersonPage.tsx:68` repeat `Row`'s
  heading; the rec-dot rule is in `SearchPage.tsx:90` and `Guide.tsx:159`;
  `ChannelHit` (`SearchPage.tsx:127`) repeats the guide's channel cell
  (`Guide.tsx:117`); `LibraryPage.tsx:30` `ActiveChip` and `FilterBar`'s
  chips (`FilterBar.tsx:78`) are the same thing. Share each, in `components/livetv/ChannelBits`
  and `ui.tsx`.

## P14 · Keyboard: tabs, radios, menus

- `Segmented` (`ui.tsx:98`), `Choices` (`FilterMenu.tsx:65`), the
  hand-rolled group in `LibraryManagePage.tsx:111` and `MergeTitles`'
  native radios are four `role="radio"` styles; none handles arrow keys.
  One component, roving tabindex, Left/Right moves.
- `MenuButton` (`ui.tsx:224,230`): buttons need `type="button"`; picking
  an item or clicking outside should return focus to the trigger (Escape
  already does). `QueueMenu.tsx` is a `role="dialog"` without `useDialog`.
- `ActivityPage.tsx:189`: the running indicator is a `StatusPill` with
  `label=""` and a `title` on a wrapper; Pause and Square are icon-only
  with `title` alone. Give them `aria-label`s and the pill `sr-only` text.
- `Sidebar.tsx:99` has `aria-label` on a plain `div`; `SearchPage.tsx:138`
  on a lucide icon. Move them to elements with a role, or drop them.
- `index.css:114`: the global `:focus-visible` outline is unlayered, and
  the `outline: none` in `.poster-link`/`.still-link`/`.title-link`
  (`:333,345,366`) sits inside `@layer components`. Unlayered rules win, so
  posters get both the outline and the accent border. Lift those rules out
  of the layer (confirm in a browser first).
- `.guide-cell-dim` at `opacity-25` (`index.css:484`) is far under AA;
  decide whether it's decorative (then `aria-hidden` the text) or raise it.

## P15 · Search: see more, locked channels explained

**Scope** (`pages/SearchPage.tsx`)
- `:15` caps each group at 20 and `:44`'s "N found" counts the capped
  total. Ask for the real total (`/api/search` returns `total` per kind)
  and add "Show all N" per row, which switches that row to a `PosterGrid`.
- `:141`: a copy-protected channel hit isn't focusable and only an icon's
  `aria-label` explains it ("Encrypted", while the guide says
  "Copy-protected"). Render it as a disabled row with "Copy-protected"
  text, as the Channels tab does.

## P16 · Small smells

- `AdminMenus.tsx:89-90`: a stale "beside Mark watched" JSDoc stacked on
  `ExtraActions`, and `TitleActions` has none.
- `LibraryPage.tsx:49` writes the module-level `saved` map during render;
  do it in an effect.
- `LibraryPage.tsx:175` counts sort in the filter badge but shows no chip
  for it (`:257`).
- `ActivityPage.tsx:103` strips the first word of a job label with a
  regex; have the server send `kind` and `target` apart.
- `Recordings.tsx:142` re-implements `Meter` with a critical tint; give
  `Meter` a `tone`.
- `TwoStep.tsx:129`: Cancel fires a disable request and swallows its error.
  `LinkPage.tsx:45` renders a blank name when there's no profile.
- `ItemPage.tsx` is 516 lines: P1, P3 and P5 take `Seasons`, `Extras`,
  `MatchPanel` and `FilesCard` out, which does the split.

**What's already right, and the plan leans on**: `attempt()`/`notify()`,
`useDialog`, `PageHeader`/`EmptyState`, `router.back(fallback)`,
`PosterGrid`'s virtualising and scroll memory, `LibraryPage`'s remembered
filters, the sidebar `SearchBox` (debounce, replace-navigation, `/`),
`SearchPage` keeping the last results while typing, `FilterMenu`'s sheet /
dropdown split, the safe-area and `.touch-target` classes, the player's
keyboard handling, and the admin-aware copy on Home and the status bar.
