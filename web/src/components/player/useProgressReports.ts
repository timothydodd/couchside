import { useEffect, type RefObject } from "react";
import { api } from "../../lib/api";

const REPORT_EVERY_MS = 10_000;

/**
 * Saves the resume point while a file plays: every 10 seconds, on pause, when
 * the tab is hidden, and when playback ends or the page goes. The reports
 * also tell the server who's watching what, and how (mode), for Settings.
 */
export function useProgressReports(videoRef: RefObject<HTMLVideoElement | null>, fileId: number, modeRef: RefObject<string>) {
  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    // The last real position. By the time this effect's cleanup sends the
    // final report, the source effect has already reset the element.
    let last = { position: 0, duration: 0 };
    const remember = () => {
      if (v.duration && isFinite(v.duration)) last = { position: v.currentTime, duration: v.duration };
    };
    const report = (keepalive = false, stopped = false) => {
      remember();
      const { position, duration } = last;
      if (!duration || position < 1) return;
      const state = stopped ? "stopped" : v.paused ? "paused" : "playing";
      const body = { position, duration, state, mode: modeRef.current };
      const url = `/api/files/${fileId}/progress`;
      // While the page lives, api() renews an expired token and retries;
      // a keepalive report on the way out can't wait for that.
      const sent = keepalive
        ? fetch(url, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), keepalive })
        : api(url, { method: "PUT", json: body });
      void sent.catch(() => {});
    };
    const t = setInterval(() => !v.paused && report(), REPORT_EVERY_MS);
    v.addEventListener("timeupdate", remember);
    const onPause = () => report();
    const onHide = () => document.visibilityState === "hidden" && report(true);
    const onPageHide = () => report(true, true);
    v.addEventListener("pause", onPause);
    document.addEventListener("visibilitychange", onHide);
    window.addEventListener("pagehide", onPageHide);
    return () => {
      clearInterval(t);
      report(true, true);
      v.removeEventListener("timeupdate", remember);
      v.removeEventListener("pause", onPause);
      document.removeEventListener("visibilitychange", onHide);
      window.removeEventListener("pagehide", onPageHide);
    };
  }, [fileId, videoRef, modeRef]);
}
