import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type HlsType from "hls.js";
import PlayerFrame from "../components/player/PlayerFrame";
import { useProgressReports } from "../components/player/useProgressReports";
import { useTextSubtitles } from "../components/player/useTextSubtitles";
import { InfoRows, type SettingSection } from "../components/player/SettingsMenu";
import { ApiError, api, useApi } from "../lib/api";
import { fmtClock, fmtResolution } from "../lib/format";
import { errText } from "../lib/errors";
import { hlsEngine } from "../lib/hls";
import { previewFrame, type TrickIndex } from "../lib/trickplay";
import { chooseSource, fmtMbps, hlsCopyCaps, presetById, presetSource, presetsFor, sourceKey, stepDown, type Quality, type Source } from "../lib/playback";
import { audioLabel, subtitleDetail, subtitleLabel, type AudioTrack, type SubtitleTrack } from "../lib/tracks";
import { PROBLEM_TEXT, type MarkedSegment, type BreakMode, type Commercials, type HlsSession, type PlayInfo, type Segment } from "../lib/types";
import { BREAK_MODES, sameLanguage } from "../lib/prefs";
import { useIsAdmin } from "../stores/auth";
import { usePrefs, useProfile } from "../stores/profile";
import { useRouter } from "../stores/router";

const STALL_MIN_MS = 1500;
const STALL_WINDOW_MS = 60_000;
const STALLS_TO_STEP = 3;
const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];
const BROADCAST = new Set(["ts", "mpg", "mpeg", "wtv"]); // containers worth offering commercial detection for


type Sub = { kind: "off" } | { kind: "text"; track: SubtitleTrack } | { kind: "burn"; track: SubtitleTrack };

// Where to start the next file when moving between a movie's parts, instead
// of its saved position.
let partStart: { fileId: number; at: number } | null = null;

/**
 * Movies and episodes. Plays the original file when the browser can, then an
 * optimized copy, then a server stream. Picking another audio track or
 * picture subtitles switches to a server stream that includes them.
 */
export default function PlayerPage({ fileId }: { fileId: number }) {
  // Fresh, never cached: the resume point is read once from this.
  const { data: loaded, error: infoError, reload: reloadInfo } = useApi<PlayInfo>(`/api/files/${fileId}`, { fresh: true });
  // useApi hands back the previous file's info for a render after fileId changes.
  const info = loaded?.fileId === fileId ? loaded : undefined;
  const { data: streams, error: streamsError } = useApi<{ audio: AudioTrack[]; subtitles: SubtitleTrack[] }>(`/api/files/${fileId}/streams`);
  const { data: marks, reload: reloadMarks } = useApi<{ segments: MarkedSegment[] }>(`/api/files/${fileId}/segments`);
  // Seek-bar thumbnails, when this file's library makes them (404 otherwise).
  const { data: trick } = useApi<TrickIndex>(`/api/files/${fileId}/trickplay`);
  const [breakPoll, setBreakPoll] = useState(false);
  const { data: comm, reload: reloadComm } = useApi<Commercials>(`/api/files/${fileId}/commercials`, { pollMs: breakPoll ? 5000 : undefined });
  const prefs = usePrefs();
  const isAdmin = useIsAdmin();
  const breakMode = prefs.commercials ?? "auto";
  const { go, back } = useRouter();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<HlsType | null>(null);

  const [quality, setQuality] = useState<Quality>("auto");
  const [forceHls, setForceHls] = useState(false);
  const [stepHeight, setStepHeight] = useState(0);
  const [audio, setAudio] = useState<number | null>(null); // null = file's default track
  const [sub, setSub] = useState<Sub>({ kind: "off" });
  const [rate, setRate] = useState(1);
  const [nonce, setNonce] = useState(0);
  const [session, setSession] = useState<HlsSession | null>(null);
  const [fatal, setFatal] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [noticeAction, setNoticeAction] = useState<{ label: string; onClick: () => void } | null>(null);
  const [loading, setLoading] = useState<string | null>("Loading…");

  const startAt = useRef<number | null>(null);
  const started = useRef(false);
  const stalls = useRef<number[]>([]);
  const waitingSince = useRef<number | null>(null);
  const recovered = useRef({ session: false, media: false });

  const exit = useCallback(() => back(info ? `/item/${info.itemId}` : "/"), [back, info]);

  const defaultAudio = streams?.audio.find((a) => a.default)?.index ?? 0;
  const burnIndex = sub.kind === "burn" ? sub.track.index : null;
  const needServer = (audio !== null && audio !== defaultAudio) || burnIndex !== null;

  const source: Source | null = useMemo(() => {
    if (!info || info.problem) return null;
    if (needServer) return quality === "auto" ? { kind: "hls", height: stepHeight } : presetSource(quality);
    return chooseSource(info, quality, forceHls, stepHeight);
  }, [info, quality, forceHls, stepHeight, needServer]);
  // A server stream is made for one audio track, so one for "the default
  // track" waits until the track list says which that is (or has failed to
  // load): made sooner, it would play track 0 while the menu shows the default.
  const audioKey = audio ?? (source?.kind !== "hls" ? "d" : streams ? `d${defaultAudio}` : streamsError ? "d0" : "wait");
  const key = source ? `${sourceKey(source)}:${audioKey}:${burnIndex ?? "n"}:${nonce}` : null;

  useEffect(() => {
    startAt.current = null;
    started.current = false;
    stalls.current = [];
    recovered.current = { session: false, media: false };
    setForceHls(false);
    setStepHeight(0);
    setAudio(null);
    setSub({ kind: "off" });
    setFatal(null);
  }, [fileId]);

  useEffect(() => {
    if (info && startAt.current === null) {
      if (partStart?.fileId === info.fileId) {
        startAt.current = partStart.at;
        partStart = null;
        return;
      }
      const ok = !info.watched && info.positionSec > 30 && (!info.durationSec || info.positionSec < info.durationSec - 30);
      startAt.current = ok ? info.positionSec : 0;
    }
  }, [info]);

  // Once streams are known, turn on text subtitles in the profile's language
  // (a full track over a forced one), or else any forced track.
  const subtitleLang = prefs.subtitleLang ?? "";
  useEffect(() => {
    const text = streams?.subtitles.filter((s) => s.text) ?? [];
    const mine = text.filter((s) => sameLanguage(s.language, subtitleLang));
    const pick = mine.find((s) => !s.forced) ?? mine[0] ?? text.find((s) => s.forced);
    if (pick) setSub({ kind: "text", track: pick });
  }, [streams, subtitleLang]);

  const flash = (msg: string, action: { label: string; onClick: () => void } | null = null) => {
    setNotice(msg);
    setNoticeAction(() => action);
    setTimeout(() => setNotice((n) => (n === msg ? null : n)), 7000);
  };

  const switchTo = useCallback((apply: () => void) => {
    const v = videoRef.current;
    if (v && v.currentTime > 0) startAt.current = v.currentTime;
    stalls.current = [];
    apply();
  }, []);

  // --- attach the source ---------------------------------------------------------------
  useEffect(() => {
    const v = videoRef.current;
    if (!v || !info || !source || !key || audioKey === "wait") return;
    let cancelled = false;
    let hls: HlsType | null = null;
    let sessionId: string | null = null;
    setFatal(null);
    setSession(null);
    setLoading(source.kind === "hls" ? "Preparing stream…" : "Loading…");
    const closeOnUnload = () => {
      if (sessionId) void fetch(`/api/hls/${sessionId}`, { method: "DELETE", keepalive: true });
    };
    window.addEventListener("pagehide", closeOnUnload);
    const seekOnLoad = () => {
      const t = startAt.current ?? 0;
      if (t > 0 && Math.abs(v.currentTime - t) > 1) v.currentTime = t;
      void v.play().catch(() => {});
    };

    if (source.kind !== "hls") {
      v.src = source.url;
      v.addEventListener("loadedmetadata", seekOnLoad, { once: true });
    } else {
      (async () => {
        let s: HlsSession;
        try {
          s = await api<HlsSession>(`/api/files/${info.fileId}/hls`, {
            method: "POST",
            json: { height: source.height, bitrateK: source.bitrateK ?? 0, ...hlsCopyCaps(), audioIndex: audio ?? defaultAudio, burnSubtitle: burnIndex ?? -1 },
          });
        } catch (e) {
          if (!cancelled) {
            setFatal(e instanceof ApiError && e.status === 429 ? `${e.message}. Stop another stream or try again shortly.` : String((e as Error).message ?? e));
            setLoading(null);
          }
          return;
        }
        if (cancelled) return void fetch(`/api/hls/${s.sessionId}`, { method: "DELETE", keepalive: true });
        sessionId = s.sessionId;
        setSession(s);
        const engine = await hlsEngine("streaming video");
        if (cancelled) return;
        if ("error" in engine) return setFatal(engine.error);
        if ("native" in engine) {
          v.src = s.playlist;
          v.addEventListener("loadedmetadata", seekOnLoad, { once: true });
          return;
        }
        const { Hls } = engine;
        const policy = {
          maxTimeToFirstByteMs: 90_000,
          maxLoadTimeMs: 120_000,
          timeoutRetry: { maxNumRetry: 2, retryDelayMs: 0, maxRetryDelayMs: 0 },
          errorRetry: { maxNumRetry: 3, retryDelayMs: 1000, maxRetryDelayMs: 8000 },
        };
        const player = new Hls({ startPosition: startAt.current ?? 0, maxBufferLength: 30, maxMaxBufferLength: 90, fragLoadPolicy: { default: policy } });
        hls = player;
        hlsRef.current = player;
        player.on(Hls.Events.ERROR, (_e, data) => {
          if (!data.fatal) return;
          if (data.type === Hls.ErrorTypes.MEDIA_ERROR && !recovered.current.media) {
            recovered.current.media = true;
            player.recoverMediaError();
            return;
          }
          const code = data.response?.code;
          // A copied-video stream can end a little before the playlist says
          // (416 from the server), or fail in its last stretch: treat as the end.
          const near = isFinite(v.duration) && v.duration - v.currentTime < 90;
          if (data.type === Hls.ErrorTypes.NETWORK_ERROR && (code === 416 || near)) {
            v.pause();
            void onEndedRef.current();
            return;
          }
          if (data.type === Hls.ErrorTypes.NETWORK_ERROR && code === 404 && !recovered.current.session) {
            recovered.current.session = true;
            switchTo(() => setNonce((n) => n + 1));
            return;
          }
          setFatal(code ? `The server stream failed (HTTP ${code}). Check Activity or the server log.` : `Playback failed: ${data.details}`);
        });
        player.loadSource(s.playlist);
        player.attachMedia(v);
        player.once(Hls.Events.MANIFEST_PARSED, () => void v.play().catch(() => {}));
      })();
    }
    return () => {
      cancelled = true;
      window.removeEventListener("pagehide", closeOnUnload);
      v.removeEventListener("loadedmetadata", seekOnLoad);
      hls?.destroy();
      hlsRef.current = null;
      closeOnUnload();
      v.removeAttribute("src");
      v.load();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  // keep speed and subtitle visibility across source switches
  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    v.playbackRate = rate;
    const show = () => {
      for (const t of Array.from(v.textTracks)) t.mode = t.label === "couchside" && sub.kind === "text" ? "showing" : "disabled";
    };
    show();
    v.addEventListener("loadedmetadata", show);
    const t = setTimeout(show, 800); // hls.js may reset text tracks after attaching
    return () => {
      v.removeEventListener("loadedmetadata", show);
      clearTimeout(t);
    };
  }, [rate, sub, key]);

  // --- text subtitles --------------------------------------------------------------------
  const textTrack = sub.kind === "text" ? sub.track : null;
  useTextSubtitles(videoRef, fileId, textTrack);

  // --- errors and stalls ---------------------------------------------------------------
  const onVideoError = () => {
    const v = videoRef.current;
    if (!source || !v?.error) return;
    if (source.kind === "hls") {
      if (!v.src.startsWith("blob:")) setFatal("The stream couldn't be decoded by this browser.");
      return;
    }
    const code = v.error.code;
    if (code === MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED || code === MediaError.MEDIA_ERR_DECODE) {
      flash("This browser can't play this file's format directly, so the server is converting it.");
      switchTo(() => setForceHls(true));
    } else {
      setFatal("The file couldn't be loaded. It may have moved; try rescanning the library.");
    }
  };
  const onWaiting = () => {
    const v = videoRef.current;
    if (!v || v.seeking || !started.current) return;
    waitingSince.current = performance.now();
  };
  const onPlaying = () => {
    started.current = true;
    setLoading(null);
    const since = waitingSince.current;
    waitingSince.current = null;
    if (since == null || quality !== "auto" || !source || !info) return;
    const now = performance.now();
    if (now - since < STALL_MIN_MS) return;
    stalls.current = [...stalls.current.filter((t) => now - t < STALL_WINDOW_MS), now];
    if (stalls.current.length < STALLS_TO_STEP) return;
    const next = stepDown(source, info.height);
    if (next == null) return;
    flash(`Playback kept buffering, so it switched to ${next}p. Pick a quality in Settings to override.`);
    switchTo(() => {
      setForceHls(true);
      setStepHeight(next);
    });
  };

  // --- progress ------------------------------------------------------------------------
  // Reports also tell the server who's watching what, and how (Settings shows it).
  const modeRef = useRef("");
  modeRef.current = describe(source, session);
  useProgressReports(videoRef, fileId, modeRef);

  // --- parts: a movie split across files plays as one timeline -----------------------
  const parts = useMemo(() => {
    const list = info?.parts;
    const at = list?.findIndex((p) => p.fileId === fileId) ?? -1;
    if (!list || list.length < 2 || at < 0) return null;
    const starts: number[] = [];
    let t = 0;
    for (const p of list) {
      starts.push(t);
      t += p.durationSec ?? 0;
    }
    return { list, at, starts, total: t };
  }, [info, fileId]);
  const playPartAt = useCallback(
    (t: number) => {
      if (!parts) return;
      let j = 0;
      while (j + 1 < parts.starts.length && parts.starts[j + 1] <= t) j++;
      const next = parts.list[j];
      if (next.fileId === fileId) return;
      partStart = { fileId: next.fileId, at: Math.max(0, t - parts.starts[j]) };
      go(`/play/${next.fileId}`, { replace: true });
    },
    [parts, fileId, go],
  );

  const onEnded = async () => {
    const v = videoRef.current;
    // Not awaited: moving on straight away means a Back pressed now can't be
    // overtaken by a late jump to the next episode.
    if (v?.duration) void api(`/api/files/${fileId}/progress`, { method: "PUT", json: { position: v.duration, duration: v.duration, state: "stopped" } }).catch(() => {});
    if (parts && parts.at + 1 < parts.list.length) {
      // The rest of the same movie: always carry on, from the start of the next part.
      partStart = { fileId: parts.list[parts.at + 1].fileId, at: 0 };
      go(`/play/${parts.list[parts.at + 1].fileId}`, { replace: true });
    } else if (info?.nextFileId && !parts && prefs.autoplayNext !== false) go(`/play/${info.nextFileId}`, { replace: true });
    else exit();
  };
  const onEndedRef = useRef(onEnded);
  onEndedRef.current = onEnded;

  // --- commercials -----------------------------------------------------------------------
  const detecting = comm?.status === "queued" || comm?.status === "running";
  useEffect(() => setBreakPoll(detecting), [detecting]);
  const findCommercials = () =>
    void api(`/api/files/${fileId}/commercials`, { method: "POST" })
      .then(reloadComm)
      .catch((e) => flash(`Couldn't start commercial detection: ${(e as Error).message}`));
  // A detected break that's really show: hidden for everyone, until undone.
  const notCommercial = (b: Segment) => {
    const set = (restore: boolean) => api(`/api/files/${fileId}/commercials/dismissed`, { method: "POST", json: { ...b, restore } }).then(reloadComm);
    const span = `${fmtClock(b.start)}–${fmtClock(b.end)}`;
    void set(false)
      .then(() =>
        flash(`${span} is no longer marked as a commercial.`, {
          label: "Undo",
          onClick: () => {
            setNotice(null);
            void set(true).catch((e) => flash(`Couldn't undo: ${(e as Error).message}`));
          },
        }),
      )
      .catch((e) => flash(`Couldn't change the break: ${(e as Error).message}`));
  };
  const chooseBreakMode = (m: BreakMode) => useProfile.getState().setPrefs({ commercials: m });

  // --- intro and credits -----------------------------------------------------------------
  const intro = marks?.segments.find((s) => s.kind === "intro");
  const credits = marks?.segments.find((s) => s.kind === "credits");
  const mark = async (what: string) => {
    const v = videoRef.current;
    if (!v) return;
    const t = v.currentTime;
    const url = (kind: string) => `/api/files/${fileId}/segments/${kind}`;
    try {
      if (what === "intro-start") await api(url("intro"), { method: "PUT", json: { start: t, end: intro && intro.end > t + 1 ? intro.end : t + 60 } });
      else if (what === "intro-end") await api(url("intro"), { method: "PUT", json: { start: intro && intro.start < t - 1 ? intro.start : Math.max(0, t - 60), end: t } });
      else if (what === "credits") await api(url("credits"), { method: "PUT", json: { start: t, end: isFinite(v.duration) ? v.duration : t + 3600 } });
      else await api(url(what === "clear-intro" ? "intro" : "credits"), { method: "DELETE" });
      await reloadMarks();
      flash(what.startsWith("clear") ? "Mark cleared." : "Marked. Everyone watching this file gets the skip button.");
    } catch (e) {
      flash(`Couldn't save the mark: ${errText(e)}`);
    }
  };

  // --- settings ------------------------------------------------------------------------
  const mode = describe(source, session);
  const qualityLabel = (q: Quality) => (q === "auto" ? "Auto" : (presetById(q)?.label ?? q));
  const settings: SettingSection[] = [
    {
      id: "quality",
      label: "Quality",
      value: quality === "auto" ? `Auto${session?.height && source?.kind === "hls" ? ` (${session.height}p)` : info?.height ? ` (${fmtResolution(info.width, info.height)})` : ""}` : qualityLabel(quality),
      options: [
        {
          id: "auto",
          label: "Auto",
          detail: "Original when your browser can play it; steps down if it keeps buffering",
          active: quality === "auto",
        },
        ...presetsFor(info?.height).map((p) => ({ id: p.id, label: p.label, detail: fmtMbps(p.bitrateK), active: p.id === quality })),
      ],
      onSelect: (id) =>
        switchTo(() => {
          const q = id as Quality;
          setQuality(q);
          if (q === "auto") {
            setForceHls(false);
            setStepHeight(0);
          }
        }),
    },
    {
      id: "audio",
      label: "Audio",
      hidden: !streams || streams.audio.length < 2,
      value: streams?.audio.find((a) => a.index === (audio ?? defaultAudio)) ? audioLabel(streams.audio.find((a) => a.index === (audio ?? defaultAudio))!) : "",
      options: streams?.audio.map((a) => ({
        id: String(a.index),
        label: audioLabel(a),
        detail: [a.title, a.index !== defaultAudio && source?.kind !== "hls" ? "Switches to a server stream" : ""].filter(Boolean).join(" · ") || undefined,
        active: a.index === (audio ?? defaultAudio),
      })),
      onSelect: (id) => switchTo(() => setAudio(Number(id) === defaultAudio ? null : Number(id))),
    },
    {
      id: "subtitles",
      label: "Subtitles",
      hidden: !streams || streams.subtitles.length === 0,
      value: sub.kind === "off" ? "Off" : subtitleLabel(sub.track),
      options: [
        { id: "off", label: "Off", active: sub.kind === "off" },
        ...(streams?.subtitles ?? []).map((s) => ({
          id: s.key,
          label: subtitleLabel(s),
          detail: subtitleDetail(s),
          active: sub.kind !== "off" && sub.track.key === s.key,
        })),
      ],
      onSelect: (id) => {
        const t = streams?.subtitles.find((s) => s.key === id);
        // Text subtitles overlay any source; leaving burned-in subs changes the
        // stream, so keep the position when coming from them.
        const fromBurn = sub.kind === "burn";
        if (!t) return fromBurn ? switchTo(() => setSub({ kind: "off" })) : setSub({ kind: "off" });
        if (t.text) return fromBurn ? switchTo(() => setSub({ kind: "text", track: t })) : setSub({ kind: "text", track: t });
        switchTo(() => setSub({ kind: "burn", track: t }));
      },
    },
    commercialsSection(comm, breakMode, !!info && BROADCAST.has(info.container), chooseBreakMode, findCommercials, isAdmin),
    {
      // Admins mark where the intro and credits are, at the playhead.
      id: "marks",
      label: "Intro and credits",
      hidden: !isAdmin,
      value: [intro && "Intro", credits && "Credits"].filter(Boolean).join(", ") || "Not marked",
      options: [
        { id: "intro-start", label: "The intro starts here", detail: intro ? `Now ${fmtClock(intro.start)} to ${fmtClock(intro.end)}` : undefined },
        { id: "intro-end", label: "The intro ends here" },
        { id: "credits", label: "The credits start here", detail: credits ? `Now at ${fmtClock(credits.start)}` : undefined },
        ...(intro ? [{ id: "clear-intro", label: "Clear the intro mark" }] : []),
        ...(credits ? [{ id: "clear-credits", label: "Clear the credits mark" }] : []),
      ],
      onSelect: (id) => void mark(id),
    },
    {
      id: "speed",
      label: "Playback speed",
      value: rate === 1 ? "Normal" : `${rate}×`,
      options: SPEEDS.map((r) => ({ id: String(r), label: r === 1 ? "Normal" : `${r}×`, active: r === rate })),
      onSelect: (id) => setRate(Number(id)),
    },
    {
      id: "info",
      label: "Playback info",
      value: mode,
      content: <PlaybackInfo info={info} session={session} source={source} videoRef={videoRef} hlsRef={hlsRef} />,
    },
  ];

  const problem = info?.problem ? PROBLEM_TEXT[info.problem].detail : null;
  const errorText = problem ?? fatal ?? infoError;

  return (
    <PlayerFrame
      videoRef={videoRef}
      title={info?.title ?? " "}
      subtitle={info?.subtitle}
      timeline={
        parts
          ? { kind: "vod", parts: { offset: parts.starts[parts.at], total: parts.total, starts: parts.starts.slice(1), seek: playPartAt } }
          : { kind: "vod" }
      }
      settings={settings}
      breaks={comm?.status === "done" ? comm.segments : undefined}
      segments={marks?.segments}
      introMode={prefs.intros ?? "button"}
      onCredits={
        parts
          ? undefined
          : info?.nextFileId
            ? { label: "Next episode", go: () => void onEnded() }
            : { label: "Skip credits", go: () => void onEnded() }
      }
      preview={trick ? (t) => (info?.durationSec && t > info.durationSec ? null : previewFrame(fileId, trick, t)) : undefined}
      breakMode={breakMode}
      onBack={exit}
      loading={errorText ? null : loading}
      notice={notice}
      noticeAction={notice ? noticeAction : null}
      onNotCommercial={isAdmin && comm?.status === "done" ? notCommercial : undefined}
      error={errorText ? { title: "Can't play this right now", message: errorText, actions: <button className="btn-primary" onClick={() => (info ? switchTo(() => setNonce((n) => n + 1)) : void reloadInfo())}>Try again</button> } : null}
      videoProps={{
        onError: onVideoError,
        onWaiting,
        onPlaying,
        // Ready but not playing: the browser refused to start it by itself
        // (a page reload has no click behind it). Show the play button, not
        // "Preparing stream…" for ever.
        onCanPlay: () => videoRef.current?.paused && setLoading(null),
        onSeeking: () => (waitingSince.current = null),
        onEnded: () => void onEnded(),
      }}
    />
  );
}

/**
 * The Commercials settings page: how to treat breaks once they're known,
 * otherwise detection status and a button to start it. Only shown for
 * broadcast recordings, or files that have been through detection.
 */
function commercialsSection(
  comm: Commercials | undefined,
  mode: BreakMode,
  broadcast: boolean,
  onMode: (m: BreakMode) => void,
  onFind: () => void,
  canRedo: boolean, // running detection again is for admins
): SettingSection {
  const hidden = !comm || (comm.status === "none" && !(broadcast && comm.available));
  if (comm?.status === "done") {
    const n = comm.segments.length;
    return {
      id: "commercials",
      label: "Commercials",
      hidden,
      value: n === 0 ? "None found" : `${n} break${n === 1 ? "" : "s"} · ${BREAK_MODES.find((m) => m.id === mode)!.short}`,
      options: [
        ...BREAK_MODES.map((m) => ({ id: m.id, label: m.label, detail: m.detail, active: m.id === mode })),
        ...(comm.available && canRedo ? [{ id: "again", label: "Look again", detail: "Re-run detection on this file" }] : []),
      ],
      onSelect: (id) => (id === "again" ? onFind() : onMode(id as BreakMode)),
    };
  }
  const text =
    comm?.status === "queued"
      ? "Waiting for its turn on the server…"
      : comm?.status === "running"
        ? "Looking for commercial breaks. They'll appear on the timeline when it's done."
        : comm?.status === "failed"
          ? `Detection failed: ${comm.error || "unknown error"}`
          : "This file hasn't been checked for commercials yet.";
  return {
    id: "commercials",
    label: "Commercials",
    hidden,
    value: comm?.status === "failed" ? "Failed" : comm?.status === "none" ? "Not checked" : "Finding…",
    content: (
      <div className="px-3 pb-3 pt-1 text-xs text-content-muted">
        <p>{text}</p>
        {comm?.available && (comm.status === "none" || comm.status === "failed") && (
          <button className="btn-primary mt-3" onClick={onFind}>
            {comm.status === "failed" ? "Try again" : "Find commercials"}
          </button>
        )}
      </div>
    ),
  };
}

function describe(source: Source | null, s: HlsSession | null): string {
  if (!source) return "";
  if (source.kind === "direct") return "Direct play";
  if (source.kind === "optimized") return "Optimized copy";
  if (!s) return "Starting…";
  if (s.mode === "remux") return "Direct stream";
  return `Transcoding ${s.height}p`;
}

function PlaybackInfo({
  info,
  session,
  source,
  videoRef,
  hlsRef,
}: {
  info?: PlayInfo;
  session: HlsSession | null;
  source: Source | null;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  hlsRef: React.RefObject<HlsType | null>;
}) {
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);
  const v = videoRef.current;
  const q = v?.getVideoPlaybackQuality?.();
  let ahead = 0;
  if (v) for (let i = 0; i < v.buffered.length; i++) if (v.buffered.start(i) <= v.currentTime && v.buffered.end(i) >= v.currentTime) ahead = v.buffered.end(i) - v.currentTime;
  const bw = hlsRef.current?.bandwidthEstimate;
  const rows: [string, React.ReactNode][] = [
    ["Mode", describe(source, session) || "–"],
    ["Source", info ? [info.container.toUpperCase(), info.videoCodec.toUpperCase(), fmtResolution(info.width, info.height), info.audioCodec.toUpperCase()].filter(Boolean).join(" · ") : "–"],
  ];
  if (source?.kind === "hls" && session) {
    rows.push(["Video", session.copyVideo ? "Copied (no re-encode)" : `H.264 ${session.height}p${session.bitrateK ? ` · ${(session.bitrateK / 1000).toFixed(1)} Mbps` : ""}${session.hdr ? " · HDR→SDR" : ""}`]);
    rows.push(["Audio", session.copyAudio ? "Copied" : "AAC stereo"]);
    if (session.burnSub >= 0) rows.push(["Subtitles", "Burned in by the server"]);
    rows.push([
      "Transcoder",
      session.copyVideo
        ? "–"
        : session.hw && session.hw !== "none"
          ? `${session.hw.toUpperCase()}${session.hw === "vaapi" ? (session.hwDecode ? " · GPU decode" : " · CPU decode") : ""}`
          : "CPU (software)",
    ]);
  }
  rows.push(["Playing", v && v.videoWidth ? `${v.videoWidth}×${v.videoHeight}` : "–"]);
  rows.push(["Buffered", `${ahead.toFixed(0)}s ahead`]);
  if (q) rows.push(["Dropped frames", `${q.droppedVideoFrames} of ${q.totalVideoFrames}`]);
  if (bw) rows.push(["Bandwidth", `${(bw / 1e6).toFixed(1)} Mbps`]);
  return <InfoRows rows={rows} />;
}
