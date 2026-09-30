import { useEffect, useState, type RefObject } from "react";

export interface MediaState {
  paused: boolean;
  ended: boolean;
  waiting: boolean;
  time: number;
  duration: number; // NaN / Infinity for live
  bufferedEnd: number; // end of the buffered range around the playhead
  seekStart: number;
  seekEnd: number;
  volume: number;
  muted: boolean;
  rate: number;
  width: number;
  height: number;
}

const EVENTS = [
  "play", "pause", "playing", "waiting", "seeking", "seeked", "timeupdate", "durationchange", "progress",
  "volumechange", "ratechange", "ended", "loadedmetadata", "emptied", "canplay",
];

function read(v: HTMLVideoElement): MediaState {
  let bufferedEnd = 0;
  for (let i = 0; i < v.buffered.length; i++) {
    if (v.buffered.start(i) <= v.currentTime + 0.5 && v.buffered.end(i) >= v.currentTime) bufferedEnd = v.buffered.end(i);
  }
  const s = v.seekable.length ? v.seekable.start(0) : 0;
  const e = v.seekable.length ? v.seekable.end(v.seekable.length - 1) : 0;
  return {
    paused: v.paused,
    ended: v.ended,
    waiting: v.readyState < 3 && !v.paused,
    time: v.currentTime,
    duration: v.duration,
    bufferedEnd,
    seekStart: s,
    seekEnd: e,
    volume: v.volume,
    muted: v.muted,
    rate: v.playbackRate,
    width: v.videoWidth,
    height: v.videoHeight,
  };
}

/** Live snapshot of a <video> element's playback state. */
export function useMediaState(ref: RefObject<HTMLVideoElement | null>): MediaState {
  const [state, setState] = useState<MediaState>(() => ({
    paused: true, ended: false, waiting: true, time: 0, duration: NaN, bufferedEnd: 0, seekStart: 0, seekEnd: 0,
    volume: 1, muted: false, rate: 1, width: 0, height: 0,
  }));
  useEffect(() => {
    const v = ref.current;
    if (!v) return;
    const update = () => setState(read(v));
    EVENTS.forEach((e) => v.addEventListener(e, update));
    const t = setInterval(update, 1000); // live ranges grow without events
    update();
    return () => {
      EVENTS.forEach((e) => v.removeEventListener(e, update));
      clearInterval(t);
    };
  }, [ref]);
  return state;
}
