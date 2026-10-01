import { useEffect, useRef, useState, type RefObject } from "react";

const CUSHION_SEC = 6; // buffered ahead before (re)starting
const MAX_HOLD_MS = 15_000; // a slow stream still starts eventually

/** Seconds buffered ahead of the playhead. */
export function bufferedAhead(v: HTMLVideoElement): number {
  for (let i = 0; i < v.buffered.length; i++) {
    if (v.buffered.start(i) <= v.currentTime + 0.5 && v.buffered.end(i) >= v.currentTime) return v.buffered.end(i) - v.currentTime;
  }
  return 0;
}

/**
 * Live streams arrive in real time, so a player that starts (or resumes) as
 * soon as one segment is in plays a second, stalls, plays a second. This
 * holds playback until a few seconds are buffered: at the start, and after
 * any stall. One short pause instead of constant stutter, and the cushion
 * absorbs later hiccups. holding is true while it waits. Reset with key
 * (e.g. the channel) whenever a new stream starts.
 */
export function useLiveCushion(videoRef: RefObject<HTMLVideoElement | null>, key: unknown) {
  const [holding, setHolding] = useState(true);
  const since = useRef(performance.now());
  const ours = useRef(false); // we paused it, so we resume it (a viewer's pause stays)

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    since.current = performance.now();
    ours.current = true;
    setHolding(true);
    const hold = () => {
      if (v.seeking || v.paused || ours.current) return;
      ours.current = true;
      since.current = performance.now();
      setHolding(true);
      v.pause();
    };
    const check = () => {
      if (!ours.current) return;
      if (bufferedAhead(v) >= CUSHION_SEC || performance.now() - since.current > MAX_HOLD_MS) {
        ours.current = false;
        setHolding(false);
        void v.play().catch(() => {});
      } else if (!v.paused) {
        v.pause(); // autoplay or hls.js started it early
      }
    };
    const onPlay = () => {
      // The viewer pressed play during a hold (a click or key just happened,
      // unlike autoplay): respect it.
      if (ours.current && navigator.userActivation?.isActive) {
        ours.current = false;
        setHolding(false);
      }
    };
    const t = setInterval(check, 500);
    v.addEventListener("waiting", hold);
    v.addEventListener("progress", check);
    v.addEventListener("play", onPlay);
    return () => {
      clearInterval(t);
      v.removeEventListener("waiting", hold);
      v.removeEventListener("progress", check);
      v.removeEventListener("play", onPlay);
    };
  }, [videoRef, key]);

  return holding;
}
