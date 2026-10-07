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
| P1  | Episodes as a grid, list on phones                      | Feature  | S    |      |
| P2  | Episode page: plot, still, guest stars, crew            | Feature  | M    |      |
| P3  | Picture plays, everything else opens the page           | Feature  | S    |      |
| P4  | Rows that scroll: cast, Home, search                    | Fix      | S    | Done |
| P5  | Edit a title from its page; Manage is library settings  | Feature  | M    |      |
| P6  | Couchside's own confirm and prompt dialogs              | Fix      | S    | Done |
| P7  | Shared badge, empty, loading and error blocks           | Cleanup  | S    |      |
| P8  | Back buttons and page headers behave alike              | Fix      | S    |      |
| P9  | Phone layout: rows that crowd or run off                | Fix      | S    |      |
| P10 | Home: the hero plays, and says why it's empty           | Fix      | S    |      |
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

## P1 · Episodes as a grid, list on phones

**Why.** `Seasons` (`pages/ItemPage.tsx:304`) lists episodes as rows with a
176px still and one line of text, so a 22-episode season is a long scroll
and most of the width is empty on a desktop. A grid of 16:9 cards shows a
whole season at once, which is also how the player's Continue Watching tile
already looks (`ContinueCard`, `components/Rows.tsx:36`).

**Scope**
- A `EpisodeCard` component (`components/EpisodeCard.tsx`): the still (or
  `.poster-placeholder`) in a `.still`, the progress strip, the watched tick
  and problem badge as `.art-badge` overlays (top right, like `PosterCard`),
  and under it `E4 · Title` in `.poster-title` and date, runtime and rating
  in `.poster-meta`. Date-named recordings show the air date in place of
  the number, as `EpisodeItem` does now.
- `Seasons` renders a CSS grid of cards from `md` up:
  `grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4`. No virtualiser:
  a season is at most a few hundred cards, and the page scrolls as a whole.
- Below `md` (`usePhone()` in `lib/media.ts`), keep the list, but build it
  from the same card in a horizontal layout (`EpisodeCard` takes
  `layout="row"`), so the two share one set of badges and text.
- `Extras` (`:363`) uses the same component. Its caption is "Extra title"
  with the movie's name dropped (the page heading already says it).
- Delete `StillRow` and `EpisodeItem` once nothing uses them.
- The season tabs (`navtab`) stay. Scroll the active tab into view when the
  page opens on a later season (`firstUnwatched`).

**Done when** a 24-episode season fits on a 1440px screen in two or three
rows, and the phone list looks the same as today.

## P2 · Episode page: plot, still, guest stars, crew

**Why.** Clicking an episode plays it; there's nowhere to read what it's
about or who's in it. The `episodes` table holds only title, date, rating
and IMDb id (`db/migrations/0001_init.sql:39`), and `tmdbSeason`
(`metadata/tmdb.go:200`) drops TMDB's `overview`, `still_path`, `runtime`,
`guest_stars` and `crew` when it decodes the season. The still shown today is
a frame grab at 25% (`worker.still`, `worker/match.go:301`).

**Scope, server (P2a)**
- Migration 0021: `episodes.plot TEXT`, `runtime_min INTEGER`,
  `still_url TEXT` (`db.ImageURL`, so it's served through the remote image
  cache), `tmdb_id INTEGER`.
- `metadata.Episode` and `db.EpisodeMeta` carry the four. `tmdbSeason`
  decodes `overview`, `still_path` (full URL via the image base), `runtime`,
  `guest_stars` (name, character, profile_path, tmdb person id) and `crew`
  (filtered by `crewJobs`, so Director and Writer). OMDb keeps what it has.
- Per-episode credits: `item_credits` gets a nullable `episode_id`
  (migration 0021; index on it). `SetCredits` for an episode replaces its
  rows; `ApplyEpisodeMeta` is the caller. People are the same `people` rows,
  so `/api/people/{id}` and `PersonPage` list the episodes a guest is in
  (add `episodes` to `PersonDetail`, with the show's title and `S1 · E4`).
- `GET /api/episodes/{id}`: `{episode: EpisodeRow + plot, runtimeMin,
  stillUrl, imdbId, tmdbId; series: ItemSummary; files: MediaFile[]
  (every copy, with the manage fields); cast: CreditRow[] (guest stars);
  crew: CreditRow[]; prev, next: {id, season, episode, title} | null}`.
- `EpisodeRow` gains `plot` (so the card can show a tooltip or a two-line
  clamp later) and `hasProviderStill`.
- The still: when `still_url` is set, the artwork job for the series
  downloads it to `$CACHE/files/<id>/still.webp` (same path the frame grab
  uses; `files.has_still` already flags it) and the frame grab is skipped.
  A re-match with the same URL doesn't download again (`src` sidecar, as
  `poster.src` does). Keep the grab as the fallback for files TMDB doesn't
  know.
- Existing libraries pick this up on the next match (`Re-match` in
  Libraries, or the 3-day TMDB TV cache expiring); no backfill job.

**Scope, web (P2b)**
- Route `/episode/{id}` (`stores/router.ts`), `pages/EpisodePage.tsx`.
  Layout mirrors `ItemPage`: the still as the hero (`.hero-art` +
  `.hero-fade`, the show's backdrop when there's no still), a back button
  to the show's page on the right season, then `Show name` as a `title-link`
  above `S2 · E4 · Title`, date · runtime · rating · the file's quality
  chip, Play / Resume (`btn-primary`) and Mark watched, the plot,
  `CrewLine`, `CastRow` labelled "Guest stars", then a `FilesCard` when
  there's more than one copy, and Previous / Next episode links.
- The title page's season tabs are reachable from the episode page's back
  button with `?season=N` so `Seasons` opens on it.
- `SearchPage` episode hits (`ContinueCard`) get the page as their title
  link once it exists.

**Done when** an episode of a TMDB-matched show has a plot, TMDB's still,
guest stars with photos, and Director and Writer, and a guest star's
person page lists the episode.

## P3 · Picture plays, everything else opens the page

**Why.** Today the whole episode row is one `Link` to the player
(`StillRow`, `ItemPage.tsx:384`). With P2 there are two destinations, and
`ContinueCard` already draws the line: the picture resumes, the title opens
the page.

**Scope**
- In `EpisodeCard`, the `.still` is a `Link` to `/play/{fileId}` with the
  hover play button and `aria-label="Play S2 E4 Title"`; the caption block
  is a `Link` to `/episode/{id}` with `title-link`. Nothing else on the card
  is clickable, so a mis-click between them does nothing. An episode with a
  `problem` has no play link; its picture opens the page too, where the
  problem is explained (`PROBLEM_TEXT`).
- The same split on `PosterCard`? No: a poster opens the title page, as
  today. Only 16:9 tiles (episode, extra, Continue Watching) play from the
  picture. Say so in `docs/style.md` under a short "What a click does" note.
- Keyboard: both links are in the tab order; the play link's focus ring is
  the `.still-link` border, the caption's the `title-link` colour.

**Done when** clicking the still plays, clicking the title opens the
episode page, and Tab reaches both.

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

## P5 · Edit a title from its page; Manage is library settings

**Why.** The title page's "⋯" (`components/item/AdminMenus.tsx`
`TitleActions`) already has Edit details (title, year, rating, genres,
description), scan, optimize, intros, commercials and delete, and each
episode and extra has its own "⋯". What it can't do is change the poster or
backdrop, search for the right match (only paste an id: `MatchPanel`,
`pages/ItemPage.tsx:220`), mark a movie file as a part or a copy, or delete
one copy of a movie. Those live only in `ItemPanel`
(`components/manage/ItemPanel.tsx`) behind `/libraries/{id}`, reached by
"Manage in library" and then finding the title again in a table. One place
to edit a title, and the Manage view left to what's about the library.

**Scope**
- `GET /api/items/{id}/manage` (admin) returns the item's `ManageRow`
  (`db.ManageRows` already computes it per library; add a one-item
  variant), so `ItemPanel` can open from an item id without the table.
- The title page (and P2's episode page) gets an admin `btn-ghost` "Edit"
  (`Pencil`) beside Mark watched, opening `ItemPanel` as the drawer it
  already is. Its sections, in order: Details (the `EditDetails` fields,
  inline rather than a second modal), Match (`MetadataSearch`, with the
  id/URL box from `MatchPanel` as its second row), Artwork
  (`ArtworkEditor`), Files (`FileList`: roles, best copy, delete file,
  delete the optimized copy). The jobs and Delete stay in the "⋯" menu,
  where they are now; drop the panel's own `ItemActions` and `DeleteItem`.
  The panel takes `itemId`, loads its own row, and `onChanged` reloads the
  page behind it.
- Remove from `ItemPage`: `MatchPanel` (keep the one-line "Unmatched ·
  read from files as …" status, whose "Fix match" opens the panel on
  Match), and the delete-optimized button in `FilesCard`. `FilesCard`
  stays for everyone as a read-only "which copy plays" table; hide it when
  there's one file with nothing notable.
- `TitleActions`' "Manage in library" becomes "Library settings".
- `LibraryManagePage` becomes **Library settings**: name and path
  (`PUT /api/libraries/{id}`, today only in the `LibrariesPage` form),
  Scan, Optimize all, Re-match, Delete library, and the stats line. The
  table stays below as "Titles" (quality, duplicates, unmatched, SD are
  still the way to find what needs fixing; the multi-select Merge and
  Delete stay), but a row's title is a `Link` to `/item/{id}?edit=1`,
  which opens the title page with the panel open. Drop the `?item=` open
  path (`LibraryPage.tsx:152` links there; point it at the same URL).
- `LibrariesPage`'s split button keeps Scan; its menu loses what moved.
- Phones: the drawer is already full-width (`max-w-md`); check the Files
  section's role `select` and title input wrap.

**Done when** a wrong poster, a wrong match, a duplicate copy and a
misfiled extra can all be fixed from the title page without visiting
Libraries, and `/libraries/{id}` edits the library's own settings.

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

## P7 · Shared badge, empty, loading and error blocks

**Scope**
- `.badge` exists (`index.css:538`) but only `components/manage` uses it.
  Hand-rolled copies with differing padding: `LibrariesPage.tsx:82`,
  `ProgramDialog.tsx:86-99`, `Recordings.tsx:81`, `Sidebar.tsx:43,199`,
  `MobileNav.tsx:30,127,129`, `Guide.tsx:130,164` (9px and 10px bold),
  `AccountManager.tsx:127-141`, `SessionList.tsx:67`,
  `LiveTvSettings.tsx:126`, and the `tint-* rounded px-1.5 py-0.5
  text-[11px] font-semibold` spans in `ItemPage.tsx` (`StillRow`,
  `PlaybackChip`, `MatchPanel`'s Unmatched). Use `.badge`. (`HomePage.tsx:106`
  "Just added" and `FilterMenu.tsx:26`'s count match the grep but aren't
  badges; leave them.)
- `EmptyState` is sometimes in a `.card` (`LibrariesPage.tsx:56`,
  `ActivityPage.tsx:72`) and sometimes bare; icon sizes are 40, 36, 34 and
  32. Give `EmptyState` the card and one icon size, and an optional
  `action`. `LibraryPage.tsx:279` ("Nothing matches those filters") gets a
  Reset action; `SearchPage.tsx:57`, `Guide.tsx:92,94` get an icon.
- A `Loading` component (`ui.tsx`) for the three spinner wrappers
  (`h-full`, `flex-1`, `py-16`: `HomePage.tsx:24`, `PersonPage.tsx:16`,
  `ItemPage.tsx:24`, `LibraryPage.tsx:270`, `Recordings.tsx:26`,
  `Guide.tsx:51`, `Channels.tsx`, `SearchPage.tsx:53`,
  `LibraryManagePage.tsx`). No loading state at all: `LibrariesPage.tsx:14`,
  `Backups.tsx`, `SingleSignOn.tsx`; `TwoStep.tsx:68` returns null on a
  load error too, so the section silently disappears. `ActivityPage.tsx:74`
  draws an empty table before data.
- Inline result blocks that should be `ErrorNote`/`WarningNote` (or a
  `GoodNote` to add): `SingleSignOn.tsx:256` (a `text-good`/`text-critical`
  span), `TwoStep.tsx:140` (hand-rolled `tint-warning`),
  `LinkPage.tsx:63` (hand-rolled `tint-good`), `ItemPanel.tsx:83`.
- `ErrorNote` placement: `gutter pt-4` everywhere (`ActivityPage.tsx:67`,
  `Recordings.tsx:57`, `Guide.tsx:87`, `LibrariesPage.tsx:45`,
  `LibraryManagePage.tsx:108` differ).
- `Backups.tsx:99` and `Recordings.tsx:264` tables have no `thead`;
  `Backups`' Delete chip lacks the critical hover the other delete chips
  have.

**Done when** `grep -rn "text-\[1[01]px\] font-\(semi\)\?bold" web/src` finds
only `index.css`, the Home eyebrow and the filter count.

## P8 · Back buttons and page headers behave alike

**Scope**
- Back buttons: `PersonPage.tsx:44` (`btn-quiet`, own row, falls back to
  `/`), `ItemPage.tsx:90` (`btn-ghost` over the backdrop), `LibraryManage-
  Page.tsx:98` (a "Libraries" link that ignores `router.back`),
  `SignInPage.tsx:83`. One `BackButton` in `ui.tsx` taking `fallback`,
  with a `variant="overlay"` for hero pages; `PersonPage` gets the hero
  treatment (a blurred photo, like a title with no backdrop) so it matches
  `ItemPage` and P2's episode page.
- `LibraryPage.tsx:161` and `PersonPage.tsx:48` hand-roll headers; use
  `PageHeader`. `PersonPage`'s 128px photo beside a `text-3xl` name has no
  phone sizing.
- `LiveTvPage.tsx:44` tabs lack `aria-current`; `SettingsPage.tsx:43` has
  it. `.navtab-active` adds `border-b-2` with no base border
  (`index.css:265`), so the active tab is 2px taller: give `.navtab` a
  transparent border.

## P9 · Phone layout: rows that crowd or run off

**Scope**
- `LiveTvPage.tsx:49`: `guideError` sits in the tab row with `ml-auto`
  inside `overflow-x-auto`; a long message pushes the tabs off. Put it
  under the tabs as an `ErrorNote`.
- `Recordings.tsx:134`: the "Recording now" card packs text, a bar and
  three buttons in one non-wrapping row. Stack below `sm`.
- `LibrariesPage.tsx:76`: the library row's stats and split button wrap
  unevenly. Two rows below `sm`: name and path, then stats and actions.
- `ActivityPage.tsx:61`: three stat tiles always; the "Running" tile's
  `sub` is a full job label and `StatTile` doesn't truncate. Two columns
  below `sm`, truncate the sub.
- `SettingsPage.tsx:41`: the phone tab bar hides its scrollbar with no
  cue; P4's edge fade covers it.

## P10 · Home: the hero plays, and says why it's empty

**Scope**
- `HomePage.tsx:117`: the hero's "Watch" button links to `/item/{id}`,
  the same as "Details". Make it play (`/play/{fileId}`: `Home` needs the
  item's `playFileId`, or fetch `/api/items/{id}` as `:99` already does
  for the plot and take the first feature file / next episode as
  `ItemPage` does). Label it "Play" or "Resume 12:30".
- `:99` fetches the whole item for the plot, and `pickFeatured` (`:47`)
  runs on every 15s poll, so the hero swaps (and fetches again) whenever
  something new arrives. Pick the hero once per mount (seeded by the day), and have `/api/home` return the plot and play
  file for the hero so the extra fetch goes.
- `:51-58`: with libraries but nothing scanned yet, Home is stat tiles
  only (on phones, a 16px gap). Show an `EmptyState`: "Scanning…" with
  the job count when a scan is queued or running, otherwise "No titles
  yet" with Scan (admin) or "ask your admin".
- `:65`: the Unmatched tile's "Set TMDB_API_KEY to match" (admins only
  now) is env-var advice on the home page. Link to Settings → Metadata.

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
