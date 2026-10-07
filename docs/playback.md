# Playback and transcoding

Movies, episodes, live TV and in-progress recordings share one player.

## How a file is played

1. **Direct play.** If the browser can decode the file, it streams as is, with
   byte-range seeking.
2. **Optimized copy.** If there's a browser-friendly MP4 made earlier, that.
3. **Live HLS stream.** Otherwise the server converts while you watch:
   - H.264 video with audio browsers can't play (DTS, AC-3, TrueHD), or in an
     MKV, is *remuxed*: the video is copied and only the audio is converted.
   - Everything else is transcoded to H.264/AAC. HDR is tone-mapped to SDR.

Settings → Playback info in the player shows which of these is happening,
the source and output codecs, the transcoder, buffer, dropped frames and
bandwidth.

## Streams

- **Seeking.** Keyframes are forced every 4 seconds, so the whole timeline is
  seekable at once. Jumping ahead restarts ffmpeg at that point.
- **Throttling.** ffmpeg pauses once it's two minutes ahead of the player and
  stops when nobody is watching. `COUCHSIDE_MAX_TRANSCODES` caps sessions;
  idle ones are evicted to make room.
- **Quality presets** in the gear menu, like Plex: 1080p at 20, 12, 10 or 8
  Mbps, 720p at 4, 3 or 2 Mbps, 480p and 360p.
- **Buffering fallback.** In Auto quality, three stalls within a minute step
  playback down: direct, then 1080p, 720p, 480p.

## Audio and subtitles

- **Audio tracks.** Picking a non-default track switches to a server stream
  that carries it, because browsers can't change tracks in a direct-played file.
- **Text subtitles** (embedded SRT, ASS, mov_text, and sidecars next to the
  video such as `Movie.en.srt`, `.vtt`, `.ass`) are converted to WebVTT and
  cached. Embedded tracks load in 90-second chunks around the playhead, so a
  big file on a NAS doesn't have to be read end to end first. Forced tracks,
  and tracks in your profile's subtitle language, turn on automatically.
- **Picture subtitles** (Blu-ray PGS, DVD) are burned in by the server, scaled
  to the video.

## Player

- A seek bar with buffered ranges and hover times; split movies show one bar
  across all their parts ([library.md](library.md#parts-and-extras)).
- Skip back 10 seconds and forward 30, volume, full screen.
- Keys: Space or K play/pause, arrows seek and change volume, M mute,
  F full screen, S skip a commercial.
- Gear menu: Quality, Audio, Subtitles, Commercials, Playback speed,
  Playback info.
- Resume position, watched state, Continue Watching, and next-episode autoplay
  (a profile setting).

## Optimize

Background encodes make browser-friendly MP4 copies of a title or a whole
library, so they direct-play later with no live transcoding. They run one at a
time (`COUCHSIDE_ENCODE_WORKERS`), up to `COUCHSIDE_OPTIMIZE_HEIGHT`, show
progress on the Activity page, and can be cancelled.

## Hardware

- **Picking the encoder.** `COUCHSIDE_HWACCEL=auto` (the default outside the
  container) tries the encoders this machine might have and keeps the first
  that passes a one-frame test encode: NVENC then Quick Sync on Windows; VAAPI
  (when `/dev/dri` exists), NVENC then Quick Sync on Linux. The container and
  Helm chart set `none`; set it to `vaapi` there.
- **VAAPI (Intel/AMD iGPUs).** Set `COUCHSIDE_HWACCEL=vaapi` and pass
  `/dev/dri` into the container (with a privileged container or a GPU device
  plugin, and the host's render group as a supplemental group). Decoding,
  scaling, HDR tone mapping and encoding all run on the GPU, so a 4K HEVC HDR
  film converts with little CPU. The Intel drivers are in the image.
- **NVENC and Quick Sync** encode on the GPU; decoding, scaling and tone
  mapping still run on the CPU. They need an ffmpeg built with them: the
  Windows installer includes one, and Alpine's (in the image) lacks NVENC and
  the newer Intel QSV runtime. NVENC also needs an NVIDIA driver new enough for
  the ffmpeg build (upstream 8.1+ builds want 610 or later).
- **Fallbacks.** An unusable GPU falls back to software. Settings → System
  shows what passed its start-up test, or ffmpeg's reason for each encoder that
  didn't. A file the GPU can't decode falls back to CPU decoding.

## Skipping intros and credits

When Couchside knows where an episode's intro is, the player shows **Skip
intro** while it plays (or press S); during the end credits it offers **Next
episode**. Your settings choose a button (the default), skipping
automatically, or neither.

It knows from, in this order: an admin marking it in the player (Settings →
Intro and credits, at the playhead); the file's own chapters, when they're
named (Intro, Opening, End Credits and the like); or, for TV libraries with
**Find intros** ticked, by comparing the sound of each season's episodes for
the opening they share. That last one can also be run for one show from the
"⋯" menu on its page.
