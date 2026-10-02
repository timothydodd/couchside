# Live TV and DVR

Broadcast TV needs an [HDHomeRun](https://www.silicondust.com) tuner: set
`COUCHSIDE_HDHOMERUN` to its IP address. The channel lineup refreshes hourly.
[Your own channels](#your-own-channels), made from your library, need no tuner.

## Guide

- Listings come from SiliconDust's free guide service, about a day ahead,
  refreshed every 3 hours.
- The Guide and Channels tabs share a filter bar:
  - Search by channel name, number or show title.
  - Genres from the guide's categories, and **New** for first airings.
  - HD-only or SD-only.
  - **Hide weak** drops channels under 60% signal quality at the tuner's last
    scan; each channel shows signal bars.
  - **Hide locked** hides DRM channels.
  - The star pins a channel to the top; **Favorites** shows only pinned ones.
    Favourites are per profile; filters are remembered per browser.

## Watching

- **Browsers** get the tuner stream deinterlaced and transcoded to HLS at
  1080p, 720p or 480p (on the GPU when VAAPI is set up). Viewers of the same
  channel share one tuner, released about 20 seconds after the last leaves.
- **TVs** (the Roku app) play broadcasts they can decode (MPEG-2 or H.264 with
  AC-3, most channels) straight off the tuner: no server work and full
  broadcast quality. **Settings → Your settings → On TVs** turns this off for
  slow connections.
- **Timeline.** The seek bar spans the program you're watching in clock time
  (9:00–10:00 at 9:30 sits in the middle), or the half-hour slot when the
  guide has nothing. Time before you tuned in and not yet aired is hatched.
  Pause and rewind within the session (up to 3 hours); **GO LIVE** jumps back.
- **Smooth playback.** The player stays about 8 seconds behind live and waits
  for a few seconds of video before starting or after a stall, so a hiccup
  means one short pause instead of constant stutter. Playback info shows the
  server's encoding speed and warns when it can't keep up in real time.
- Page Up and Page Down change channel. A show being recorded offers
  **Start over**.

## Recording

- Record any program from the guide, the channel list or the live player.
  Recordings copy the tuner stream with no transcoding.
- **Padding:** 10 seconds before and after by default
  (Settings → Advanced, or `COUCHSIDE_DVR_PAD_BEFORE` / `_AFTER`). Changing it
  also updates recordings that haven't started.
- **Names** are Plex-style (`Show/Season 3/Show - S03E15 - Title.ts`, or by
  air date).
- **Where they go:** Settings → Live TV & DVR. Couchside's own storage, any TV
  library folder, or another folder under the media root. Recordings reuse a
  matching show and season folder, so they join that show instead of
  duplicating it, and existing recordings can be moved along. Recording into a
  library needs the media mounted writable.
- **Same-titled shows.** The guide doesn't say which *MacGyver* is airing (1985
  or 2016). When a recording is scheduled, the episode is checked against TMDB
  (or OMDb), and a show whose title is shared gets the year in its folder
  (`MacGyver (2016)/Season 2/…`).
- **Watching in progress:** **From start** on the Recordings tab plays a
  recording that's still being made from its beginning, with a jump to live.
- **Robustness.** A dropped signal or busy tuner is retried and the parts are
  joined. A server restart mid-recording resumes into the same file.
  Overlapping recordings beyond the tuner count are flagged as conflicts.
- **Permissions.** Recording is a per-account switch an admin turns on; people
  allowed to record can change only their own recordings and series.

## Series recordings

Choose **Record series** on any program and pick a mode:

- **Episodes I don't have:** skips episodes already in the linked library show
  or already recorded, cancelled airings and repeats. Airings with no episode
  info are recorded only when they're new.
- **New episodes only:** first airings only.
- **Every airing:** everything, repeats included.

Rules can be limited to one channel and keep only the last N recordings. A
rule links to the same-named library show whose episode titles match upcoming
airings, and you can change the link. Rules re-run after every guide refresh
and hourly; when every tuner is booked, a later airing of the same episode is
used instead.

## Commercial skipping

- Each finished recording is checked for breaks with
  [Comskip](https://github.com/erikkaashoek/Comskip), built into the container
  image. The recording itself is never cut.
- Breaks are marked on the timeline. The player skips them automatically, with
  a **Watch it** link to go back. A profile can switch to a skip button (or the
  S key) or turn skipping off.
- Skipping starts 1 second into a break and stops 1 second before its end, so
  it never cuts into the show (Settings → Advanced).
- Other `.ts` files, such as older Plex DVR recordings, can be checked from the
  player's Commercials page, or a whole show at once: on its page, the **⋯**
  menu has **Find commercials in every episode** (only episodes not checked
  yet) and **Check every episode again**. Admins only; the jobs show on the
  Activity page.
- Zip installs: put `comskip` on the PATH or set `COUCHSIDE_COMSKIP`;
  `COUCHSIDE_COMSKIP_INI` points at a tuned `comskip.ini` (keep `output_edl=1`).

## Your own channels

Admins can make channels from the library in **Settings → Your channels →
New channel**. They play around the clock like broadcast TV and sit in the
guide and channel list next to tuner channels (numbered from 900 by default),
on the web and the Roku. No tuner is needed.

- **What plays:** movies, shows or both, from some or all libraries, filtered
  by genre (and genres to leave out), years and rating, or a hand-picked list
  of titles. Presets (Movie night, Sitcom marathon, A decade, One show,
  Classic TV) fill these in; the editor previews the next few hours.
- **Order:** shuffle (everything once before anything repeats), or in order
  (shows take turns, each continuing from its last episode; movies oldest
  first). Split movies play all their parts; extras never play.
- **Commercials (optional):** point a channel at a folder of clips (old ads,
  trailers, bumpers). It needn't be a library. Programs can start on the hour,
  half hour or quarter hour, padded with clips (never more than 10 minutes),
  and have breaks every 8, 12 or 20 minutes. A guide entry covers its program's
  commercials, as on real TV. `scripts/fetch-demo-media.py` downloads
  public-domain 1950s and 60s commercials to try it with.
- **How it works:** the schedule is built 36 hours ahead and topped up every 10
  minutes, so nothing shifts when Couchside restarts or the library changes.
  Nothing is encoded until someone tunes in: the stream starts part way into
  whatever is on, at the quality the player asks for, and is shared by everyone
  watching. It stops 20 seconds after the last viewer leaves.
- They can't be recorded: everything on them is already in your library.

## Limits

- ATSC 3.0 channels with DRM can't be watched or recorded outside
  SiliconDust's apps; they're listed but locked.
- The guide covers about a day. A Schedules Direct source for two weeks of
  listings isn't built yet.
