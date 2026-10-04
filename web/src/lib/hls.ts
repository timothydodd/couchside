import type HlsType from "hls.js";
import { nativeHls } from "./playback";

let mod: Promise<typeof HlsType> | null = null;

/**
 * hls.js, fetched the first time a stream is played. A failed fetch (the
 * network dropped, or the server was updated and this tab still asks for the
 * old build's file) isn't remembered, so "Try again" can succeed.
 */
export function loadHls(): Promise<typeof HlsType> {
  const p = (mod ??= import("hls.js/light").then((m) => m.default));
  p.catch(() => {
    if (mod === p) mod = null;
  });
  return p;
}

/** Shown when hls.js itself couldn't be fetched (not a browser limitation). */
const HLS_LOAD_FAILED = "Couldn't load the player. Check the connection and try again; if Couchside was just updated, reload the page.";

/** How a playlist gets played in this browser. */
export type HlsEngine =
  | { Hls: typeof HlsType } // hls.js
  | { native: true } // the browser plays HLS itself (Safari, iOS)
  | { error: string }; // neither

/**
 * Picks how to play an HLS playlist here. `what` finishes "This browser
 * can't play …" (e.g. "live streams"). The three players share this, so a
 * failed download of hls.js is never reported as a browser limitation.
 */
export async function hlsEngine(what: string): Promise<HlsEngine> {
  const Hls = await loadHls().catch(() => null);
  if (Hls?.isSupported()) return { Hls };
  if (nativeHls()) return { native: true };
  return { error: Hls ? `This browser can't play ${what}.` : HLS_LOAD_FAILED };
}
