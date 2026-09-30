export type ItemKind = "movie" | "series";
export type MatchStatus = "pending" | "matched" | "unmatched";

export interface ItemSummary {
  id: number;
  kind: ItemKind;
  title: string;
  sortTitle: string;
  year: number | null;
  genres: string[];
  rating: number | null;
  runtimeMin: number | null;
  hasPoster: boolean;
  hasBackdrop: boolean;
  matchStatus: MatchStatus;
  addedAt: number;
  updatedAt: number;
  fileCount: number;
  watchedCount: number;
  lastAddedAt: number;
}

export interface Item extends ItemSummary {
  libraryId: number;
  parsedTitle: string;
  parsedYear: number;
  plot: string;
  rated: string;
  imdbId: string;
  totalSeasons: number | null;
}

export interface MediaFile {
  id: number;
  path: string;
  size: number;
  durationSec: number | null;
  container: string;
  videoCodec: string;
  audioCodec: string;
  width: number | null;
  height: number | null;
  audioTracks: number;
  subtitleTracks: number;
  positionSec: number;
  watched: boolean;
  optimized: boolean;
  problem: FileProblem;
}

export interface EpisodeRow {
  id: number;
  season: number;
  episode: number;
  title: string;
  released: string;
  airDate: string;
  rating: number | null;
  fileId: number;
  problem: FileProblem;
  hasStill: boolean;
  durationSec: number | null;
  positionSec: number;
  watched: boolean;
}

export interface ItemDetail {
  item: Item;
  files: MediaFile[];
  seasons?: { season: number; episodes: EpisodeRow[] }[];
}

export interface PlayInfo {
  fileId: number;
  itemId: number;
  kind: ItemKind;
  title: string;
  subtitle: string;
  positionSec: number;
  durationSec: number | null;
  watched: boolean;
  hasStill: boolean;
  hasBackdrop: boolean;
  updatedAt: number;
  nextFileId: number | null;
  progress: number;
  container: string;
  videoCodec: string;
  audioCodec: string;
  width: number | null;
  height: number | null;
  optimized: boolean;
  problem: FileProblem;
}

/** A commercial break, in seconds from the start of the file. */
export interface Segment {
  start: number;
  end: number;
}

/** GET /api/files/{id}/commercials */
export interface Commercials {
  available: boolean; // comskip is installed on the server
  status: "none" | "queued" | "running" | "failed" | "done";
  error: string;
  segments: Segment[];
}

/** How the player treats commercial breaks. */
export type BreakMode = "auto" | "button" | "off";

/** Avatar colours, named after the theme tokens they use. */
export type ProfileColor = "accent" | "pink" | "cyan" | "secondary" | "good" | "warning" | "critical";

/** Per-profile preferences, stored on the server. Every key is optional. */
export interface Prefs {
  theme?: "light" | "dark" | "system";
  autoplayNext?: boolean; // default true
  commercials?: BreakMode; // default "auto"
  subtitleLang?: string; // turn on text subtitles in this language; "" = only forced ones (default)
  liveHeight?: number; // live TV and in-progress recording quality, default 720
}

export interface Profile {
  id: number;
  name: string;
  color: ProfileColor;
  prefs: Prefs;
  createdAt: number;
}

/** Set by the scanner for files that can't be played at all. */
export type FileProblem = "" | "unreadable" | "no-video";

export const PROBLEM_TEXT: Record<Exclude<FileProblem, "">, { label: string; detail: string }> = {
  unreadable: {
    label: "Damaged file",
    detail: "This file is damaged or incomplete, so it can't be played. Replace it on the NAS and rescan.",
  },
  "no-video": {
    label: "No playable video",
    detail: "This file has no video Couchside can decode. It's most likely a DRM-protected iTunes purchase.",
  },
};

export interface HlsSession {
  sessionId: string;
  playlist: string;
  mode: "remux" | "transcode";
  height: number;
  copyVideo: boolean;
  copyAudio: boolean;
  hdr: boolean;
  hw: string;
  audio: number;
  burnSub: number;
  bitrateK: number;
}

export interface TranscodeSession {
  id: string;
  fileId: number;
  title: string;
  mode: "remux" | "transcode";
  height: number;
  bitrateK: number;
  copyAudio: boolean;
  hdr: boolean;
  hw: string;
  created: string;
  positionSec: number;
  running: boolean;
  paused: boolean;
  aheadSec: number;
}

export interface Home {
  continueWatching: PlayInfo[];
  recentMovies: ItemSummary[];
  recentSeries: ItemSummary[];
}

export interface Library {
  id: number;
  name: string;
  path: string;
  kind: "movies" | "tv";
  lastScanAt: number | null;
  createdAt: number;
  itemCount: number;
  fileCount: number;
}

export interface JobCounts {
  queued: number;
  running: number;
  failed: number;
  current: string;
}

export interface Status {
  livetv: { configured: boolean; recording?: number; liveSessions?: number; online?: boolean };
  version: string;
  providers: string[];
  mediaRoot: string;
  counts: { movies: number; series: number; episodes: number; unmatched: number; libraries: number };
  jobs: JobCounts;
  scanEvery: string;
  workers: number;
  transcode?: {
    hwaccel: string;
    requested: string;
    tonemap: boolean;
    maxSessions: number;
    active: number;
    encodeWorkers: number;
    optimizeHeight: number;
  };
}

export interface Job {
  id: number;
  kind: "scan" | "match" | "artwork" | "still" | string;
  refId: number;
  label: string;
  status: "queued" | "running" | "done" | "failed";
  attempts: number;
  error: string;
  createdAt: number;
  startedAt: number | null;
  finishedAt: number | null;
  progress: number | null;
}

export interface Browse {
  path: string;
  parent: string | null;
  dirs: string[];
}

// --- Live TV / DVR ---------------------------------------------------------------

export interface TvChannel {
  number: string;
  name: string;
  affiliate: string;
  logoUrl: string;
  hd: boolean;
  drm: boolean;
  videoCodec: string;
  audioCodec: string;
  pinned: boolean;
  signalStrength: number | null;
  signalQuality: number | null;
}

export interface Program {
  id: number;
  channel: string;
  startAt: number;
  endAt: number;
  title: string;
  episodeTitle: string;
  episodeNum: string;
  synopsis: string;
  imageUrl: string;
  seriesId: string;
  originalAirdate: number | null;
  isNew: boolean;
  categories: string[];
  recordingId: number | null;
  recordingStatus: "" | "scheduled" | "recording" | "completed";
  ruleId: number | null;
}

export interface ChannelNow extends TvChannel {
  now: Program | null;
  next: Program | null;
}

export interface GuideResponse {
  start: number;
  hours: number;
  through: number;
  channels: (TvChannel & { programs: Program[] })[];
}

export interface LiveTvStatus {
  configured: boolean;
  device?: { FriendlyName: string; ModelNumber: string; DeviceID: string; FirmwareVersion: string; TunerCount: number };
  error?: string;
  guideError?: string;
  guideThrough?: number;
  guideUpdated?: number;
  tunersInUse?: number;
  tunerUsers?: string[];
  recording?: number;
  liveSessions?: number;
  recordingsDir?: string;
  libraryId?: number;
}

export interface LiveSessionInfo {
  sessionId: string;
  playlist: string;
  channel: string;
  name: string;
  height: number;
  hw: string;
  now: Program | null;
}

export interface Recording {
  id: number;
  channel: string;
  channelName: string;
  title: string;
  episodeTitle: string;
  episodeNum: string;
  synopsis: string;
  imageUrl: string;
  categories: string[];
  startAt: number;
  endAt: number;
  padBefore: number;
  padAfter: number;
  status: "scheduled" | "recording" | "completed" | "failed" | "cancelled";
  size: number;
  error: string;
  startedAt: number | null;
  finishedAt: number | null;
  fileId: number | null;
  ruleId: number | null;
}

export type RuleMode = "missing" | "new" | "all";

export interface SeriesRule {
  id: number;
  seriesId: string;
  title: string;
  imageUrl: string;
  mode: RuleMode;
  channel: string;
  mediaItemId: number | null;
  keepLast: number;
  enabled: boolean;
  lastRunAt: number | null;
  lastSummary: string;
  createdAt: number;
  libraryTitle: string | null;
  scheduled: number;
  recorded: number;
  nextAt: number | null;
}

export interface RuleSummary {
  airings: number;
  scheduled: number;
  added: number;
  alreadyHave: number;
  alreadyRecorded: number;
  duplicates: number;
  notNew: number;
  noEpisodeInfo: number;
  otherChannel: number;
  conflicts: number;
  cancelled: number;
  locked: number;
}

export interface LibraryMatch {
  id: number;
  title: string;
  year: number | null;
  fileCount: number;
  dvr: boolean;
  matching: number;
}

/** GET /api/libraries/{id}/manage: one movie or show with its file facts. */
export interface ManageRow {
  id: number;
  kind: ItemKind;
  title: string;
  year: number | null;
  parsedTitle: string;
  parsedYear: number;
  matchStatus: "pending" | "matched" | "unmatched";
  imdbId: string;
  hasPoster: boolean;
  customPoster: boolean;
  customBackdrop: boolean;
  updatedAt: number;
  addedAt: number;
  fileCount: number;
  episodeCount: number;
  size: number;
  maxHeight: number;
  minHeight: number;
  videoCodec: string;
  sameImdb: number;
}

/** GET /api/items/{id}/files */
export interface ManageFile {
  id: number;
  path: string;
  size: number;
  durationSec: number | null;
  container: string;
  videoCodec: string;
  audioCodec: string;
  width: number | null;
  height: number | null;
  problem: FileProblem;
  addedAt: number;
  season: number | null;
  episode: number | null;
  episodeTitle: string;
}

/** GET /api/items/{id}/lookup */
export interface SearchResult {
  imdbId: string;
  title: string;
  year: string;
  poster: string;
}

export interface DeleteResult {
  deleted: number;
  bytes: number;
  itemsRemoved: number[];
  keptFolders: string[];
}
