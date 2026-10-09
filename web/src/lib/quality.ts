// Quality labels for the library Manage view.
import type { Tone } from "../components/ui";

/** 4 = 4K, 3 = 1080p, 2 = 720p, 1 = SD, 0 = unknown. */
export function qualityTier(height: number | null | undefined): number {
  if (!height) return 0;
  if (height >= 1500) return 4;
  if (height >= 900) return 3;
  if (height >= 600) return 2;
  return 1;
}

export function qualityLabel(height: number | null | undefined): string {
  return ["?", "SD", "720p", "1080p", "4K"][qualityTier(height)];
}

export const qualityTone = (height: number | null | undefined): Tone =>
  (["muted", "warning", "muted", "good", "info"] as Tone[])[qualityTier(height)];

const RANGES: Record<string, string> = { dv: "Dolby Vision", hdr10: "HDR10", hlg: "HLG" };
/** "Dolby Vision", "HDR10", "HLG", or "" for SDR. */
export const rangeLabel = (r: string | undefined) => RANGES[r ?? ""] ?? "";
/** The poster badge: "DV" or "HDR" (HDR10 and HLG), "" for SDR. */
export const rangeShort = (r: string | undefined) => (r === "dv" ? "DV" : r === "hdr10" || r === "hlg" ? "HDR" : "");
/** What a file's range means for playback, for a tooltip. */
export function rangeDetail(r: string | undefined, dvProfile = 0): string {
  if (r === "dv") {
    const p = dvProfile ? ` (profile ${dvProfile})` : "";
    return dvProfile === 5
      ? `Dolby Vision${p}: only a Dolby Vision screen shows it as is; elsewhere it's converted while you watch`
      : `Dolby Vision${p}, with an HDR10 picture for screens without it`;
  }
  if (r === "hdr10") return "HDR10: shown in HDR on an HDR screen, converted to SDR elsewhere";
  if (r === "hlg") return "HLG (broadcast HDR)";
  return "";
}

const CODECS: Record<string, string> = { h264: "H.264", hevc: "HEVC", av1: "AV1", mpeg2video: "MPEG-2", vc1: "VC-1", mpeg4: "MPEG-4", vp9: "VP9" };
export const codecLabel = (c: string) => CODECS[c] ?? c.toUpperCase();

/**
 * Extra copies: more files than movies/episodes they cover. A movie's parts
 * together are one copy, and its extras (bonus material) aren't copies at all.
 */
export const extraFiles = (r: { kind: string; fileCount: number; episodeCount: number; parts: number; extras: number; editions?: number }) =>
  r.kind === "series"
    ? Math.max(0, r.fileCount - Math.max(1, r.episodeCount))
    : // Different cuts (theatrical and extended) are versions, not duplicates.
      Math.max(0, r.fileCount - r.extras - r.parts - (r.parts > 0 ? 0 : Math.max(1, r.editions ?? 1)));
