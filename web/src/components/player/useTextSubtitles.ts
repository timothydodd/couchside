import { useEffect, useRef, type RefObject } from "react";
import type { SubtitleTrack } from "../../lib/tracks";
import { parseVtt } from "../../lib/vtt";

const SUB_CHUNK = 90; // seconds per embedded-subtitle chunk (the server's SubtitleChunk)

/**
 * Shows a file's text subtitle track (chosen, or null for none) on the video.
 * Sidecar files load whole. Embedded tracks load in 90-second chunks around
 * the playhead: extracting a whole track means reading the entire file, which
 * takes minutes for a big remux on a NAS.
 */
export function useTextSubtitles(videoRef: RefObject<HTMLVideoElement | null>, fileId: number, chosen: SubtitleTrack | null) {
  const subTrack = useRef<TextTrack | null>(null);
  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    subTrack.current ??= v.addTextTrack("subtitles", "couchside");
    const track = subTrack.current;
    for (const c of Array.from(track.cues ?? [])) track.removeCue(c);
    track.mode = chosen ? "showing" : "disabled";
    if (!chosen) return;

    let cancelled = false;
    const seen = new Set<string>();
    const loaded = new Set<number>();
    const add = (body: string) => {
      if (cancelled) return;
      for (const c of parseVtt(body)) {
        const id = `${c.start.toFixed(2)}|${c.text}`;
        if (seen.has(id)) continue;
        seen.add(id);
        const cue = new VTTCue(c.start, c.end, c.text);
        cue.snapToLines = false; // sit a little above the control bar
        cue.line = 86;
        cue.lineAlign = "end";
        track.addCue(cue);
      }
    };
    const base = `/api/files/${fileId}/subtitles/${chosen.key}`;
    if (chosen.external) {
      void fetch(`${base}.vtt`).then((r) => (r.ok ? r.text() : "")).then(add).catch(() => {});
      return () => {
        cancelled = true;
      };
    }
    // A chunk that failed is tried again later, a few times, not on every
    // tick (this runs about four times a second). Never after a 404 (past
    // the end of the file) or a 422 (a track that can't be converted).
    const retryAt = new Map<number, number>();
    const tries = new Map<number, number>();
    const load = (k: number) => {
      if (k < 0 || loaded.has(k)) return;
      if (isFinite(v.duration) && k * SUB_CHUNK >= v.duration) return; // no such chunk
      if (performance.now() < (retryAt.get(k) ?? 0)) return;
      loaded.add(k);
      void fetch(`${base}.c${k}.vtt`)
        .then((r) => (r.ok ? r.text() : Promise.reject(r.status)))
        .then(add)
        .catch((status: unknown) => {
          loaded.delete(k);
          const n = (tries.get(k) ?? 0) + 1;
          tries.set(k, n);
          const final = status === 404 || status === 422 || n >= 4;
          retryAt.set(k, final ? Infinity : performance.now() + 5000 * 2 ** (n - 1));
        });
    };
    const around = () => {
      const k = Math.floor(v.currentTime / SUB_CHUNK);
      load(k);
      load(k + 1);
    };
    around();
    v.addEventListener("timeupdate", around);
    v.addEventListener("seeked", around);
    return () => {
      cancelled = true;
      v.removeEventListener("timeupdate", around);
      v.removeEventListener("seeked", around);
    };
  }, [chosen, fileId, videoRef]);
}
