// Choices for per-profile preferences, shared by Settings and the player.
import { languageName } from "./tracks";
import type { BreakMode } from "./types";

export const BREAK_MODES: { id: BreakMode; label: string; short: string; detail: string }[] = [
  { id: "auto", label: "Skip automatically", short: "Auto-skip", detail: "Jumps past each break, with a way back" },
  { id: "button", label: "Show a skip button", short: "Skip button", detail: "Press it, or S, to skip a break" },
  { id: "off", label: "Don't skip", short: "Marked only", detail: "Breaks are still marked on the timeline" },
];

/** Languages offered for "turn subtitles on in…" (ISO 639-1). */
export const SUBTITLE_LANGS = ["en", "es", "fr", "de", "it", "pt", "nl", "sv", "pl", "ru", "ja", "ko", "zh", "hi", "ar"];

/**
 * Whether a track's language tag is the preferred language. Files use 2- or
 * 3-letter codes ("en", "eng"), so both are compared by their display name.
 */
export function sameLanguage(track: string, pref: string): boolean {
  if (!track || !pref) return false;
  const t = track.toLowerCase();
  return t === pref || t.startsWith(pref + "-") || languageName(t) === languageName(pref);
}
