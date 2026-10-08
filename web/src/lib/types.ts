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
  /** On this profile's "My list". */
  inWatchlist: boolean;
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
  /** Where the details came from: tmdb, omdb, or "" when unmatched. */
  matchProvider: string;
  /** Details set by hand (Edit details); each stays through re-matches. */
  overrides?: { title?: string; year?: number; plot?: string; genres?: string[]; rated?: string };
}

/** What a movie file is to its movie: another copy, one part of a split movie, or an extra. */
export type FileRole = "copy" | "part" | "extra";

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
  hasStill: boolean;
  role: FileRole;
  partNo: number;
  extraTitle: string;
  rolePinned: boolean;
  /** Which cut ("Extended"); "" for the ordinary one. */
  edition: string;
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
  cast: CreditRow[];
  crew: CreditRow[];
  /** The copy of a movie this profile chose to watch; 0 = none chosen. */
  versionFileId: number;
}

/** One person in a title's credits. role is the character, or the job(s) for crew. */
export interface CreditRow {
  personId: number;
  name: string;
  role: string;
  hasPhoto: boolean;
}

export interface Person {
  id: number;
  name: string;
  hasPhoto: boolean;
}

/** A person's page: who they are and the library's titles and episodes they're in. */
export interface PersonDetail {
  person: Person;
  items: (ItemSummary & { roles: string[] })[];
  episodes: (EpisodeRow & { seriesId: number; seriesTitle: string; roles: string[] })[];
}

/** An episode's page. */
export interface EpisodeDetail {
  episode: EpisodeRow & { seriesId: number; plot: string; runtimeMin: number | null; imdbId: string; tmdbId: number };
  series: Item;
  /** Every copy, best first. */
  files: MediaFile[];
  /** Guest stars. */
  cast: CreditRow[];
  crew: CreditRow[];
  prev: EpisodeRef | null;
  next: EpisodeRef | null;
}

export interface EpisodeRef {
  id: number;
  season: number;
  episode: number;
  title: string;
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
  /** On Home because the episode before it was finished, not because this one was started. */
  nextUp?: boolean;
  progress: number;
  container: string;
  videoCodec: string;
  audioCodec: string;
  width: number | null;
  height: number | null;
  optimized: boolean;
  problem: FileProblem;
  role: FileRole;
  partNo: number;
  extraTitle: string;
  /** The whole movie, in order, when this file is one part of it. */
  parts?: PartRef[];
}

export interface PartRef {
  fileId: number;
  partNo: number;
  durationSec: number | null;
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

/** A file's intro or end credits (GET /api/files/{id}/segments). */
export interface MarkedSegment {
  kind: "intro" | "credits";
  start: number;
  end: number;
  source: "chapters" | "detected" | "manual";
}

/** Avatar colours, named after the theme tokens they use. */
export type ProfileColor = "accent" | "pink" | "cyan" | "secondary" | "good" | "warning" | "critical";

/** Per-profile preferences, stored on the server. Every key is optional. */
export interface Prefs {
  autoplayNext?: boolean; // default true
  commercials?: BreakMode; // default "auto"
  intros?: BreakMode; // skip intros: default "button"
  subtitleLang?: string; // turn on text subtitles in this language; "" = only forced ones (default)
  liveHeight?: number; // live TV and in-progress recording quality, default 720; 0 = as broadcast
  livePassthrough?: boolean; // TV apps play broadcasts they can decode untouched; default true
}

/** Ratings an account can be limited to, mildest first (the server's db.RatingLimits). */
export const RATING_LIMITS = ["G", "PG", "PG-13", "R"] as const;

export interface Profile {
  /** Signing in with the password also needs a code from an authenticator app. */
  twoStep?: boolean;
  /** Account manager only: the libraries it's limited to (empty = all) and its rating limit ("" = any). */
  libraries?: number[];
  maxRating?: string;
  id: number;
  name: string;
  color: ProfileColor;
  prefs: Prefs;
  createdAt: number;
  // Accounts (only meaningful when the server has them on).
  role: "admin" | "user";
  canRecord: boolean;
  disabled: boolean;
  mustChangePassword: boolean;
  hasPassword: boolean;
  /** Only an admin can set or change this account's password (a shared profile). */
  passwordLocked: boolean;
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
  /** VAAPI: decoding and scaling on the GPU too, not just encoding. */
  hwDecode?: boolean;
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

/** GET /api/system: the Settings page's live view of the server. */
export interface SystemInfo {
  stats: SystemStats;
  clients: ConnectedClient[];
  transcodes: TranscodeSession[];
  maxStreams: number;
  hwaccel: string;
  livetv: { configured: boolean; recording?: number; liveSessions?: number };
  jobs: JobCounts;
}

/** One reading in GET /api/system/history (5s apart, or 1-minute averages past an hour). */
export interface HistoryPoint {
  t: number; // unix seconds
  cpu: number;
  serverCpu: number;
  encCpu: number;
  mem: number;
  serverMem: number;
  encMem: number;
  encoders: number;
  streams: number;
}

export interface HistoryWindow {
  every: number; // seconds between points
  cores: number;
  memTotal: number;
  scope: string;
  points: HistoryPoint[];
}

/** A line of the server log, from GET /api/system/logs. */
export interface LogEntry {
  seq: number;
  t: number; // unix milliseconds
  level: "DEBUG" | "INFO" | "WARN" | "ERROR";
  msg: string;
  attrs: string;
}

/** CPU percentages are of all the cores available (cores), so 100 = fully busy. */
export interface SystemStats {
  available: boolean;
  scope: "container" | "host";
  cores: number;
  cpuPercent: number;
  memUsed: number;
  memTotal: number;
  server: ProcessGroup;
  encoders: ProcessGroup;
}

export interface ProcessGroup {
  count: number;
  cpuPercent: number;
  rss: number;
}

export interface ConnectedClient {
  profileId: number;
  name: string;
  color: ProfileColor;
  device: string;
  ip: string;
  connectedSec: number;
  you: boolean;
  playing: NowPlaying | null;
}

export interface NowPlaying {
  kind: "file" | "live" | "recording";
  fileId?: number;
  title: string;
  subtitle: string;
  positionSec: number;
  durationSec: number;
  mode: string;
  paused: boolean;
}

export interface Home {
  continueWatching: PlayInfo[];
  /** The profile's "My list", newest first. */
  watchlist: ItemSummary[];
  recentMovies: ItemSummary[];
  recentSeries: ItemSummary[];
}

export interface Library {
  /** Seek-bar preview thumbnails are made for this library's files. */
  trickplay: boolean;
  /** Intros are looked for in this (TV) library's seasons. */
  intros: boolean;
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
  /** Where the TMDB key comes from: this build's own, TMDB_API_KEY, or none. */
  tmdbKey: "builtin" | "custom" | "";
  mediaRoots: string[]; // media locations: admins only (empty for everyone else)
  counts: { movies: number; series: number; episodes: number; unmatched: number; libraries: number };
  jobs: JobCounts;
  scanEvery: string;
  workers: number;
  comskip?: boolean; // commercial detection is available
  transcode?: {
    hwaccel: string;
    requested: string; // COUCHSIDE_HWACCEL: auto, none, vaapi, qsv or nvenc
    note?: string; // why no GPU is used: each encoder tried and ffmpeg's reason
    tonemap: boolean;
    gpuDecode?: boolean;
    gpuTonemap?: boolean;
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
  /** What a finished job did: a scan's counts, and the files it skipped and why. */
  result: string;
}

// Settings → Server (GET /api/settings/server): environment variables admins can set here instead.
export interface ServerSetting {
  key: string; // the environment variable
  group: string;
  label: string;
  help: string;
  kind: "text" | "dir" | "file" | "int" | "duration" | "choice" | "bool" | "secret";
  options?: string[];
  default: string;
  placeholder?: string;
  advanced?: boolean; // shown under the page's Advanced area
  value: string | null; // saved here (never a secret's)
  env: string; // the variable's value (never a secret's)
  saved: boolean;
  envSet: boolean;
  pending: boolean; // changed since the server started
}

export interface ServerSettings {
  settings: ServerSetting[];
  pending: boolean;
  envFile: string;
  canRestart: boolean;
  startFailed: string; // a restart with the saved settings failed: it's running without them
}

// Media locations (GET /api/media/locations): where media can be.
export interface MediaLocation {
  id: number; // 0 for one from COUCHSIDE_MEDIA_ROOT
  path: string;
  share: boolean; // a network share (\\server\share)
  fromEnv: boolean;
  user: string;
  hasPassword: boolean;
  found: boolean; // the folder can be opened now
  error: string; // a share's sign-in failed
}

export interface MediaLocations {
  locations: MediaLocation[];
  shares: boolean; // this server signs in to network shares (Windows)
  envVar: string;
}

export interface Browse {
  path: string; // "" lists the starting points (drives, or /)
  parent: string | null; // "" goes back to the starting points
  dirs: { name: string; path: string }[];
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
  /** One of Couchside's own channels, played from the library (no tuner, can't be recorded). */
  virtual: boolean;
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
  /** An HDHomeRun is set up (without one, Live TV is only Couchside's own channels). */
  tuner: boolean;
  virtualChannels: number;
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
  hwDecode?: boolean;
  copyVideo?: boolean;
  copyAudio?: boolean;
  /** One of Couchside's own channels. */
  virtual?: boolean;
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
  /** Failed, but its pieces are still on disk and can be joined. */
  recoverable?: boolean;
  fileId: number | null;
  ruleId: number | null;
  /** Profile that scheduled it (or owns its series rule); 0 = admins only. */
  ownerId: number;
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
  /** Profile that made it; 0 = admins only. */
  ownerId: number;
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
  /** Different cuts among a movie's copies. */
  editions: number;
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
  parts: number;
  extras: number;
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
  role: FileRole;
  partNo: number;
  extraTitle: string;
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

// --- Search ----------------------------------------------------------------------

export interface SearchResult {
  query: string;
  movies: ItemSummary[];
  series: ItemSummary[];
  episodes: PlayInfo[];
  /** Cast and crew whose names match, from the library's credits. */
  people: Person[];
  channels: TvChannel[];
  programs: { program: Program; channel: TvChannel | null }[];
}

// --- Accounts ----------------------------------------------------------------------

export interface AuthInfo {
  enabled: boolean; // always true
  /** Picking a profile signs in (profiles with a password still ask for it). */
  passwordless: boolean;
  /** COUCHSIDE_AUTH=true on the server requires passwords. */
  passwordlessLocked: boolean;
  /** Passwordless: the profiles to pick from (without the admins while they're hidden; /api/auth?admin=1 lists those). */
  profiles: ProfileStub[];
  /** Admin accounts are left off the picker and sign in at /admin. */
  hideAdmins?: boolean;
  setupRequired: boolean;
  /** The first-run setup (your name, your media) isn't done. */
  firstRun: boolean;
  user: Profile | null;
  accessExpiresAt?: number;
  /** Seconds until the access token runs out. */
  expiresIn?: number;
  /** Profiles this browser holds a session for (switch without a password). */
  signedIn: ProfileStub[];
  /** This connection is plain HTTP from an internet address. */
  insecure?: boolean;
  /** The label of the "sign in through a provider" button, when single sign-on is set up. */
  oidc?: string;
}

export type ProfileStub = Pick<Profile, "id" | "name" | "color" | "hasPassword">;

/** What sign-in, setup and refresh return to the web (tokens travel as cookies). */
export interface SignedIn {
  user: Profile;
  accessExpiresAt: number;
  /** Seconds until the access token runs out (missing from servers before 0.13). */
  expiresIn?: number;
  sessionId: string;
}

export interface DeviceSession {
  id: string;
  profileId: number;
  client: "web" | "tv";
  device: string;
  userAgent: string;
  ip: string;
  createdAt: number;
  lastUsedAt: number;
  expiresAt: number;
  current: boolean;
}

/** What one of Couchside's own channels plays (server: livetv.VirtualConfig). */
export interface VirtualConfig {
  libraries: number[];
  kinds: ("movie" | "series")[];
  genres: string[];
  excludeGenres: string[];
  yearFrom: number;
  yearTo: number;
  minRating: number;
  items: number[];
  order: "shuffle" | "sequential";
  filler: { folder: string; align: number; breakEvery: number; breakLength: number };
  /** A stream channel: it shows this live stream (http or https) and nothing above applies. */
  stream?: { url: string } | null;
}

export interface VirtualChannel {
  id: number;
  number: string;
  name: string;
  config: VirtualConfig;
  /** Why it has no schedule, e.g. nothing matches its filters. */
  error?: string;
}

export interface VirtualOptions {
  genres: string[];
  titles: { id: number; title: string; year: number; kind: "movie" | "series"; libraryId: number }[];
  minYear: number;
  maxYear: number;
  nextNumber: string;
  mediaRoot: string;
}
