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
| P11 | Guide: time shifts show a loading state                 | Fix      | S    | Done |
| P12 | Copy: one name for things, admin-only advice to admins  | Fix      | S    | Done |
| P13 | Status is summarised once                               | Cleanup  | S    | Done |
| P14 | Keyboard: tabs, radios, menus                           | A11y     | S    |      |
| P15 | Search: see more, locked channels explained             | Fix      | S    | Done |
| P16 | Small smells                                            | Cleanup  | S    | Done |

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

## P11 · Guide: time shifts show a loading state · Done

**Done.** While a shifted window loads, the header shows a spinner beside
the new time and the old grid dims and ignores clicks (`stale`:
`loading` with data for another `start`). "Earlier" stops a day back,
where nothing is kept. The time controls sit in `FilterBar`'s slot, so
the guide no longer pulls the filter bar up over its own row.

## P12 · Copy: one name for things, admin-only advice to admins · Done

**Done.** "TV shows" everywhere (the phone tab keeps "TV" as its short
label, from the shared nav table). Live TV's and the Movies/TV empty
states link admins to Settings → Live TV or Libraries and tell others to
ask an admin. The tuner-conflict message says "other recorders", not Plex.
"Clear finished" counts failed jobs as finished. The job summary has one
wording and order (`useJobsSummary`, P13). The sidebar's "Manage" heading
is for admins; users get a visually hidden "Account" one. "More" on
phones lights up only for the pages it holds.

## P13 · Status is summarised once · Done

**Done.** `components/StatusBits.tsx` has `useJobsSummary` (active,
failed, one label), `RecBadge` and `JobsBadge`, used by the sidebar, the
phone bars and the status bar. `components/nav.ts` is the one nav table
(`NAV_MAIN`, `NAV_LIVE`, `NAV_MANAGE` with `admin` and `short`, and
`sectionOf`) for the sidebar, the bottom tabs and the More sheet. `ui.tsx`
`Section` replaces the search page's and person page's hand-rolled
headings; `RecDot` (`livetv/ChannelBits.tsx`) is the one rule for the red
dot in the guide, the channels list and search; `ActiveChip` lives in
`FilterMenu.tsx` for both the library pages and the Live TV filter bar.
The search page's channel hit keeps its own card layout; it isn't the
guide's cell.

## P14 · Keyboard: tabs, radios, menus · Done

**Done.** `radioKeys` (`lib/dialog.ts`) gives every `role="radiogroup"`
arrow keys with a roving tabindex: `Segmented`, `Choices`, and the library
settings page's filter, which is now `Segmented` with counts in its labels
(`label` takes a node). `MergeTitles` keeps native radios, which already
do this. `MenuButton`'s buttons are `type="button"`, and picking an item or
clicking away returns focus to the trigger. The queue overlay is a real
dialog (`useDialog`: focus in, Tab stays, Escape). Activity's encoder
state is announced ("Encoding", "Paused", "Idle"), the Settings sub-nav
is a labelled `role="group"`, and the global `:focus-visible` outline
moved into `@layer base`, so the poster, still and title links' own focus
styles win and a focused poster no longer shows both an outline and the
accent border. Guide cells outside the filter are `opacity-45`, pushed
back but readable.

## P15 · Search: see more, locked channels explained · Done

**Done.** Search asks for 50 per group (the server's cap) instead of 20,
and the count says "50+ found" when a group is full; the rows page with
P4's arrows. A copy-protected channel hit is a focusable card with a
"Copy-protected" badge and a plain-words label, matching the guide.

## P16 · Small smells · Done

**Done.** `LibraryPage` writes its remembered filters in an effect, and
shows a "Sorted by …" chip when the sort counts toward the filter badge.
`Meter` takes `tone="critical"` and the recording card uses it. "Link a
TV" says "you" when there's no profile name. `AdminMenus`' stale doc came
with P1, Two-step's Cancel with P7, and `ItemPage` is down to 290 lines
after P1, P2 and P5. Left as is: `ActivityPage` still trims the job kind
off the label with a regex; the server would have to send the target
apart, which isn't worth a wire change on its own.

**What's already right, and the plan leans on**: `attempt()`/`notify()`,
`useDialog`, `PageHeader`/`EmptyState`, `router.back(fallback)`,
`PosterGrid`'s virtualising and scroll memory, `LibraryPage`'s remembered
filters, the sidebar `SearchBox` (debounce, replace-navigation, `/`),
`SearchPage` keeping the last results while typing, `FilterMenu`'s sheet /
dropdown split, the safe-area and `.touch-target` classes, the player's
keyboard handling, and the admin-aware copy on Home and the status bar.
