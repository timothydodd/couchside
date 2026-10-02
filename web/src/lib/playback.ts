// Decides how the browser plays a file: direct (original file), optimized
// copy, or a live HLS transcode, based on what this browser can decode.

import type { PlayInfo } from "./types";

/** A conversion the viewer can pick, like Plex's: a height and a video bitrate. */
export interface Preset {
  id: string;
  height: number;
  bitrateK: number;
  label: string;
}

export const PRESETS: Preset[] = [
  { id: "1080-20", height: 1080, bitrateK: 20000, label: "1080p HD (High)" },
  { id: "1080-12", height: 1080, bitrateK: 12000, label: "1080p HD (Medium)" },
  { id: "1080-10", height: 1080, bitrateK: 10000, label: "1080p HD" },
  { id: "1080-8", height: 1080, bitrateK: 8000, label: "1080p HD (Low)" },
  { id: "720-4", height: 720, bitrateK: 4000, label: "720p HD (High)" },
  { id: "720-3", height: 720, bitrateK: 3000, label: "720p HD (Medium)" },
  { id: "720-2", height: 720, bitrateK: 2000, label: "720p HD" },
  { id: "480-1.5", height: 480, bitrateK: 1500, label: "480p" },
  { id: "360-0.7", height: 360, bitrateK: 700, label: "360p" },
];

/** "auto": the original when this browser can play it, else a conversion that steps down on buffering. */
export type Quality = "auto" | Preset["id"];

/** Presets worth offering for a source: nothing taller than the file (the smallest always). */
export function presetsFor(srcHeight: number | null | undefined): Preset[] {
  if (!srcHeight) return PRESETS;
  const fit = PRESETS.filter((p) => p.height <= Math.max(srcHeight, 360));
  return fit.length ? fit : PRESETS.slice(-1);
}

export const presetById = (id: string) => PRESETS.find((p) => p.id === id);

export const fmtMbps = (k: number) => `${k >= 10000 ? Math.round(k / 1000) : Number((k / 1000).toFixed(1))} Mbps`;

export type Source =
  | { kind: "direct"; url: string }
  | { kind: "optimized"; url: string }
  | { kind: "hls"; height: number; bitrateK?: number }; // height 0 = best available; no bitrate = the server's default

const probe = typeof document !== "undefined" ? document.createElement("video") : null;

// RFC 6381 codec strings for canPlayType, keyed by ffprobe codec name.
const VIDEO: Record<string, string> = {
  h264: "avc1.640028",
  hevc: "hvc1.1.6.L120.90",
  vp9: "vp09.00.10.08",
  av1: "av01.0.08M.08",
  vp8: "vp8",
};
const AUDIO: Record<string, string> = {
  aac: "mp4a.40.2",
  mp3: "mp4a.69",
  opus: "opus",
  vorbis: "vorbis",
  flac: "flac",
  ac3: "ac-3",
  eac3: "ec-3",
};
// MKV is left out on purpose: browsers that can play it don't admit it in
// canPlayType, and remuxing it server-side is cheap and reliable.
const CONTAINER: Record<string, string> = { mp4: "video/mp4", m4v: "video/mp4", mov: "video/mp4", webm: "video/webm" };

/** True when this browser should be able to play the original file as is. */
export function canDirectPlay(f: Pick<PlayInfo, "container" | "videoCodec" | "audioCodec">): boolean {
  if (!probe) return false;
  const base = CONTAINER[f.container];
  const v = VIDEO[f.videoCodec];
  if (!base || !v) return false;
  let codecs = v;
  if (f.audioCodec) {
    const a = AUDIO[f.audioCodec];
    if (!a) return false;
    codecs += `, ${a}`;
  }
  return probe.canPlayType(`${base}; codecs="${codecs}"`) !== "";
}

/** What the server may copy instead of re-encoding inside an HLS stream. */
export function hlsCopyCaps() {
  const mse = typeof MediaSource !== "undefined";
  return {
    copyVideo: mse ? MediaSource.isTypeSupported('video/mp4; codecs="avc1.640028"') : true,
    copyAudio: mse ? MediaSource.isTypeSupported('audio/mp4; codecs="mp4a.40.2"') : true,
  };
}

/** Safari (and iOS) play HLS natively; everyone else needs hls.js. */
export const nativeHls = () => !!probe && probe.canPlayType("application/vnd.apple.mpegurl") !== "";

/**
 * Pick a source. forceHls is set after the browser failed to play the file
 * directly or kept buffering; stepHeight is the auto-downgrade height.
 */
export function chooseSource(info: PlayInfo, quality: Quality, forceHls: boolean, stepHeight: number): Source {
  if (quality === "auto") {
    if (!forceHls && canDirectPlay(info)) return { kind: "direct", url: `/api/files/${info.fileId}/stream` };
    if (!forceHls && info.optimized) return { kind: "optimized", url: `/api/files/${info.fileId}/stream?version=optimized` };
    return { kind: "hls", height: stepHeight };
  }
  return presetSource(quality);
}

/** The HLS source for a preset (falls back to the server's choice for an unknown id). */
export function presetSource(id: string): Source {
  const p = presetById(id);
  return p ? { kind: "hls", height: p.height, bitrateK: p.bitrateK } : { kind: "hls", height: 0 };
}

/** Next rung down the ladder when auto mode keeps buffering, or null at the bottom. */
export function stepDown(current: Source, srcHeight: number | null): number | null {
  const ladder = [1080, 720, 480];
  if (current.kind !== "hls") {
    const top = srcHeight && srcHeight < 1080 ? ladder.find((h) => h <= srcHeight) ?? 480 : 1080;
    return top;
  }
  const h = current.height || Math.min(srcHeight ?? 1080, 1080);
  return ladder.find((l) => l < h) ?? null;
}

export function sourceKey(s: Source) {
  return s.kind === "hls" ? `hls:${s.height}:${s.bitrateK ?? 0}` : s.kind;
}
