// Decides how the browser plays a file: direct (original file), optimized
// copy, or a live HLS transcode, based on what this browser can decode.

import type { PlayInfo } from "./types";

export type Quality = "auto" | 1080 | 720 | 480;
export const QUALITIES: Quality[] = ["auto", 1080, 720, 480];

export type Source =
  | { kind: "direct"; url: string }
  | { kind: "optimized"; url: string }
  | { kind: "hls"; height: number }; // height 0 = best available

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
  return { kind: "hls", height: quality };
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
  return s.kind === "hls" ? `hls:${s.height}` : s.kind;
}
