import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import type { BreakMode, Segment } from "../../lib/types";

const END_SLACK = 0.5; // the last half second of a break counts as over
const NOTICE_MS = 8000;

/**
 * Commercial-break skipping for a vod timeline. In "auto" mode, entering a
 * break jumps to its end, unless the viewer seeked into it on purpose (allow).
 * The skip button is offered in "button" mode, and in "auto" mode for a
 * break the viewer chose to watch.
 */
export function useBreakSkip(videoRef: RefObject<HTMLVideoElement | null>, time: number, breaks: Segment[] | undefined, mode: BreakMode) {
  const allowed = useRef(new Set<number>());
  const [skipped, setSkipped] = useState<number | null>(null);

  useEffect(() => {
    allowed.current = new Set();
    setSkipped(null);
  }, [breaks]);

  const indexAt = useCallback((t: number) => breaks?.findIndex((b) => t >= b.start && t < b.end - END_SLACK) ?? -1, [breaks]);
  const current = mode === "off" ? -1 : indexAt(time);

  const seek = useCallback(
    (t: number) => {
      const v = videoRef.current;
      if (v) v.currentTime = t;
    },
    [videoRef],
  );

  useEffect(() => {
    if (mode !== "auto" || current < 0 || allowed.current.has(current) || !breaks) return;
    seek(breaks[current].end);
    setSkipped(current);
  }, [mode, current, breaks, seek]);

  useEffect(() => {
    if (skipped === null) return;
    const t = setTimeout(() => setSkipped(null), NOTICE_MS);
    return () => clearTimeout(t);
  }, [skipped]);

  /** Call before a viewer-initiated seek, so auto-skip doesn't undo it. */
  const allow = useCallback(
    (t: number) => {
      const i = indexAt(t);
      if (i >= 0) allowed.current.add(i);
    },
    [indexAt],
  );

  const canSkip = current >= 0 && (mode === "button" || allowed.current.has(current));
  return {
    breaks: mode === "off" ? undefined : breaks,
    allow,
    current: canSkip ? breaks![current] : null,
    skipped: skipped !== null && breaks ? breaks[skipped] : null,
    skip: () => {
      if (canSkip) seek(breaks![current].end);
    },
    /** Go back and watch the break that was just skipped. */
    watch: () => {
      if (skipped === null || !breaks) return;
      allowed.current.add(skipped);
      seek(breaks[skipped].start);
      setSkipped(null);
    },
  };
}
