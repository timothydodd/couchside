# Libraries and metadata

A library is a folder of movies or a folder of TV shows. Add one on the
Libraries page; Couchside scans it, matches every title, and fetches artwork.

## Scanning

- **File names.** Movies like `The.Matrix.1999.1080p.mkv` or
  `Inception (2010)/movie.mkv` (the folder's year wins when the file has none).
  TV like `Show/Season 1/Show S01E02.mkv`, `1x02`, bare `E01` files, and
  Plex-DVR date names (`Show - 2024-11-10 03 30 00 - Title.ts`).
- **Rescans** skip unchanged files, prune deleted ones, and run every 6 hours
  (`COUCHSIDE_SCAN_INTERVAL`). Unchanged files are re-parsed each time, so
  parser fixes reach existing libraries without a re-index. A scan that
  couldn't read a folder, or found an empty library folder (an unmounted
  share), removes nothing and shows as failed in Activity. Each scan's line in Activity says what it added, changed and removed, and which files it skipped and why (a TV file with no season and episode in its name, a sample clip).
- **Stream info** from ffprobe: duration, codecs, resolution and track counts.
  Files that can't be read (corrupt, or DRM-protected iTunes purchases) are
  flagged and the UI explains why they won't play.

## Metadata

- **[TMDB](https://www.themoviedb.org)** with no setup: release builds carry
  Couchside's own key (`TMDB_API_KEY` uses yours). Title, year, plot, genres,
  rating, posters, backdrops, per-season episode titles, and cast and crew.
- **OMDb** is an optional fallback (`OMDB_API_KEY`).
- **Caching.** Provider answers are cached in the database: movie lookups count
  as fresh for 30 days, TV for 3 (shows gain episodes). Once a title is matched
  its details are stored with it, so the cache only matters when a match runs.
- **Fix match.** On a title's page, or in the Manage view, search TMDB under
  any name, or paste an IMDb id, TMDB id or URL. A fixed match is pinned:
  automatic re-matches keep it.
- **Libraries → Re-match** refreshes a whole library's details, artwork and cast.

## Artwork

- Posters and backdrops from TMDB, resized to WebP. A frame from the video
  stands in when there's no backdrop; episodes and extras get a still.
- You can upload your own poster or backdrop in the Manage view.
- Everything pulled from the internet (artwork, cast photos, channel logos,
  guide images) is cached on the server, so browsers and TVs only ever talk to
  Couchside.

## Parts and extras

A movie folder often holds more than one file. Each movie file is one of:

- **A copy:** another version of the same movie. Extra copies show up as
  duplicates in the Manage view.
- **A part:** a movie split across files. The parts play back to back on one
  timeline that spans the whole movie, the next part starts automatically,
  and Resume continues in whichever part you stopped.
- **An extra:** bonus material, listed under **Extras** on the movie's page as
  "Movie - Behind the Scenes". Extras don't count toward watched or duplicates.

Scans guess from the path: `Part 2`, `cd1` or `Disc Two` make a part; an
extras folder inside the movie folder (`Featurettes/`, `Trailers/`,
`Behind The Scenes/`, `Extras/`…), a Plex suffix like `-trailer`, or words
like "Behind the Scenes" make an extra. A marker that's part of the real title
(*Deathly Hallows Part 2*) doesn't count. To correct a guess, open the movie
in the Manage view and pick *Copy*, *Part N* or *Extra* for the file. Choices
made by hand stick through rescans; marking a file as Part 2 makes the movie's
single other copy Part 1.

## One title across libraries

A movie or show is one entry however many libraries hold files for it. A
series you keep in the TV library and also record with the DVR shows once,
with the episodes from both; the Manage view of each library lists it, and a
profile limited to one of the libraries sees only that library's files. Titles
match by name and year as Couchside reads them from the folders, so a show
folder called `MacGyver (2016)` in one library and `MacGyver` in another are
two shows.

## Library management

- **Rename** a library or point it at another folder; watch history follows.
- **Manage view** (open a library from the Libraries page): one table per
  library. Sort by quality, size or date added; filter to duplicates,
  unmatched or SD titles. Open a title to fix its match, upload artwork, sort
  its files into copies, parts and extras, or delete files.
- **Select several titles** (admins) on the Movies or TV page: Ctrl-click
  (Cmd-click on a Mac) each one, or Shift-click for a range. An **Actions**
  menu appears above the grid: mark them watched or unwatched, delete them,
  or, with one selected, open it in the Manage view. Escape clears the
  selection.
- **Deleting** removes files from disk, along with subtitle sidecars and
  cached artwork. Empty folders are removed; folders that still hold other
  files (artwork, `.nfo`) are left and reported. This needs the media mounted
  writable.

## Browsing

- Home: a hero for the newest title, Continue Watching (what you're part way
  through, plus the next episode of a show whose last one you finished; the X
  on a card removes it until you watch it again), My list, and recently added
  rows.
- **My list:** the button on a title's page saves it to your profile's list,
  which is a row on Home and a filter in Movies and TV Shows.
- Movies and TV grids with search, genre and watched filters, and sorting.
- Title pages with seasons and episodes, cast (click someone to see everything
  of theirs in your library), parts and extras.
- An Activity page for background jobs and streams, with retry and cancel.

## Preview thumbnails

Tick **Preview thumbnails on the seek bar** when adding or editing a library
and the player shows a frame of the film as you move along the seek bar.
Making them reads each file from start to end once (a few minutes for a large
film on a network share), so it's off by default. The jobs run after
everything else and are listed in Activity as Thumbnails.

Switching it on makes them for the files already in the library, not only
new ones. Every scan also catches existing files up on anything they lack:
episode stills, preview thumbnails, intros, and commercial detection for
recordings. A file that failed is left alone until you retry it in Activity.

## Versions

Keep more than one copy of a film (a 4K and a 1080p, or two cuts) and its
page gets a version chooser next to Play; what you pick is remembered for
your profile. A cut is recognised from the file name: Plex's
`{edition-Final Cut}` tag, or a word after the year such as `Extended`,
`Director's Cut`, `Unrated`, `Theatrical` or `IMAX`. Different cuts aren't
counted as duplicates.
