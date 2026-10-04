import type HlsType from "hls.js";

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
export const HLS_LOAD_FAILED = "Couldn't load the player. Check the connection and try again; if Couchside was just updated, reload the page.";
