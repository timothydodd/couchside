import { useCallback, useEffect, useRef, useState } from "react";
import type HlsType from "hls.js";
import { ChevronDown, ChevronUp, CircleDot, LayoutGrid, SkipBack, Square } from "lucide-react";
import PlayerFrame, { TopButton } from "../components/player/PlayerFrame";
import { bufferedAhead, useLiveCushion } from "../components/player/useLiveCushion";
import { InfoRows, type SettingSection } from "../components/player/SettingsMenu";
import { ApiError, api, useApi } from "../lib/api";
import { fmtTime } from "../lib/format";
import { nativeHls } from "../lib/playback";
import type { ChannelNow, LiveSessionInfo, Program } from "../lib/types";
import { useProfile } from "../stores/profile";
import { useRouter } from "../stores/router";
import { useCanRecord } from "../stores/auth";

const LIVE_QUALITIES = [1080, 720, 480] as const;
let hlsModule: Promise<typeof HlsType> | null = null;
const loadHls = () => (hlsModule ??= import("hls.js/light").then((m) => m.default));

/** Live TV: the server tunes and transcodes; you can pause and rewind within the session. */
export default function LivePlayerPage({ channel }: { channel: string }) {
  const { go } = useRouter();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<HlsType | null>(null);
  const { data: channels, reload: reloadChannels } = useApi<ChannelNow[]>("/api/livetv/channels", { pollMs: 60000 });
  const [height, setHeight] = useState<number>(() => useProfile.getState().current?.prefs.liveHeight ?? 720);
  const [session, setSession] = useState<LiveSessionInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tuning, setTuning] = useState(true);
  const [recBusy, setRecBusy] = useState(false);
  const [nonce, setNonce] = useState(0);
  const [notice, setNotice] = useState<string | null>(null);
  const holding = useLiveCushion(videoRef, `${channel}:${height}:${nonce}`);
  // How fast the server adds video to the playlist, per second of wall time.
  // Below 1× it can't keep up and the stream will keep buffering whatever the player does.
  const speed = useRef<{ samples: [number, number][]; value: number | null; warned: boolean }>({ samples: [], value: null, warned: false });

  const playable = (channels ?? []).filter((c) => !c.drm);
  const idx = playable.findIndex((c) => c.number === channel);
  const current = channels?.find((c) => c.number === channel);
  const now: Program | null | undefined = current?.now ?? session?.now;

  // Programs seen on this channel this session, so the timeline can span
  // whichever one the playhead is in, even after rewinding into the last one.
  const [programs, setPrograms] = useState<Program[]>([]);
  useEffect(() => setPrograms([]), [channel]);
  useEffect(() => {
    const add = [now, current?.next].filter((p): p is Program => !!p);
    if (!add.length) return;
    setPrograms((ps) => {
      const fresh = add.filter((p) => !ps.some((q) => q.startAt === p.startAt));
      return fresh.length ? [...ps, ...fresh].slice(-6) : ps;
    });
  }, [now, current?.next]);
  // When the show ends, fetch the guide again so the next one's details arrive.
  useEffect(() => {
    if (!now) return;
    const ms = (now.endAt + 2) * 1000 - Date.now();
    if (ms <= 0 || ms > 6 * 3600_000) return;
    const t = setTimeout(() => void reloadChannels(), ms);
    return () => clearTimeout(t);
  }, [now, reloadChannels]);

  const exit = useCallback(() => go("/livetv"), [go]);
  const zap = useCallback(
    (dir: 1 | -1) => {
      if (!playable.length) return;
      go(`/watch/${playable[(idx + dir + playable.length) % playable.length].number}`, { replace: true });
    },
    [playable, idx, go],
  );

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    let cancelled = false;
    let hls: HlsType | null = null;
    let sid: string | null = null;
    setError(null);
    setTuning(true);
    setSession(null);
    setNotice(null);
    speed.current = { samples: [], value: null, warned: false };
    const leave = () => {
      if (sid) void fetch(`/api/live/${sid}`, { method: "DELETE", keepalive: true });
    };
    window.addEventListener("pagehide", leave);
    (async () => {
      let s: LiveSessionInfo;
      try {
        s = await api<LiveSessionInfo>("/api/livetv/watch", { method: "POST", json: { channel, height } });
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof ApiError ? e.message : String(e));
          setTuning(false);
        }
        return;
      }
      sid = s.sessionId;
      if (cancelled) return leave();
      setSession(s);
      const Hls = await loadHls().catch(() => null);
      if (cancelled) return;
      if (!Hls || !Hls.isSupported()) {
        if (!nativeHls()) return setError("This browser can't play live streams.");
        v.src = s.playlist;
        void v.play().catch(() => {});
        return;
      }
      const player = new Hls({
        // Sit 8s behind the newest segment (4 segments), never speed up to catch
        // up, and only jump forward when more than 40s behind.
        liveSyncDuration: 8,
        liveMaxLatencyDuration: 40,
        maxLiveSyncPlaybackRate: 1,
        maxBufferLength: 20,
        backBufferLength: 1800, // rewind up to 30 minutes within the session
        manifestLoadPolicy: {
          default: {
            maxTimeToFirstByteMs: 20000,
            maxLoadTimeMs: 30000,
            timeoutRetry: { maxNumRetry: 3, retryDelayMs: 500, maxRetryDelayMs: 2000 },
            errorRetry: { maxNumRetry: 6, retryDelayMs: 1000, maxRetryDelayMs: 4000 },
          },
        },
      });
      hls = player;
      hlsRef.current = player;
      player.on(Hls.Events.ERROR, (_e, data) => {
        if (!data.fatal) return;
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) return player.recoverMediaError();
        if (data.response?.code === 404) return setNonce((n) => n + 1);
        setError(`The live stream stopped (${data.details}). The signal may have dropped.`);
      });
      player.on(Hls.Events.LEVEL_UPDATED, (_e, data) => {
        const sp = speed.current;
        const now = performance.now() / 1000;
        sp.samples = [...sp.samples.filter(([t]) => now - t < 30), [now, data.details.totalduration]];
        const [t0, d0] = sp.samples[0];
        if (now - t0 < 15) return; // too soon after tuning to judge
        sp.value = (data.details.totalduration - d0) / (now - t0);
        if (sp.value < 0.95 && !sp.warned) {
          sp.warned = true;
          setNotice(
            `The server is encoding this channel slower than real time (${sp.value.toFixed(2)}×), so it will keep buffering. ` +
              (height > 480 ? "Pick a lower quality in Settings, or set up hardware transcoding." : "Set up hardware transcoding on the server."),
          );
        }
      });
      player.loadSource(s.playlist);
      player.attachMedia(v); // useLiveCushion starts playback once a few seconds are buffered
    })();
    return () => {
      cancelled = true;
      window.removeEventListener("pagehide", leave);
      hls?.destroy();
      hlsRef.current = null;
      leave();
      v.removeAttribute("src");
      v.load();
    };
  }, [channel, height, nonce]);

  const canRecord = useCanRecord();
  const toggleRecord = async () => {
    if (!now) return;
    setRecBusy(true);
    try {
      if (now.recordingStatus === "recording" && now.recordingId) {
        if (!confirm(`Stop recording "${now.title}"?`)) return;
        await api(`/api/dvr/recordings/${now.recordingId}/cancel`, { method: "POST" });
      } else if (!now.recordingStatus) {
        await api("/api/dvr/recordings", { method: "POST", json: { programId: now.id } });
      }
      await reloadChannels();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : String(e));
    } finally {
      setRecBusy(false);
    }
  };

  const recording = now?.recordingStatus === "recording";
  const settings: SettingSection[] = [
    {
      id: "quality",
      label: "Quality",
      value: `${height}p`,
      options: LIVE_QUALITIES.map((h) => ({ id: String(h), label: `${h}p`, detail: h === 1080 ? "8 Mbps" : h === 720 ? "4 Mbps" : "1.5 Mbps", active: h === height })),
      onSelect: (id) => {
        const h = Number(id);
        useProfile.getState().setPrefs({ liveHeight: h });
        setHeight(h);
      },
    },
    { id: "info", label: "Playback info", value: session ? `${session.height}p` : "", content: <LiveInfo session={session} videoRef={videoRef} hlsRef={hlsRef} channelName={current?.name} speed={() => speed.current.value} /> },
  ];

  return (
    <PlayerFrame
      videoRef={videoRef}
      logo={current?.logoUrl}
      badge={
        <>
          <span className="font-semibold tabular-nums text-white">{channel}</span>
          <span>{current?.name ?? session?.name}</span>
        </>
      }
      title={now ? now.title : current?.name ?? channel}
      subtitle={now ? `${fmtTime(now.startAt)} – ${fmtTime(now.endAt)}${now.episodeTitle ? ` · ${now.episodeTitle}` : ""}` : "No guide information"}
      timeline={{ kind: "live", programs }}
      liveEdge={() => hlsRef.current?.liveSyncPosition ?? null}
      settings={settings}
      onBack={exit}
      onKey={(e) => {
        if (e.key === "PageUp" || (e.key === "ArrowUp" && e.ctrlKey)) return zap(1), true;
        if (e.key === "PageDown" || (e.key === "ArrowDown" && e.ctrlKey)) return zap(-1), true;
        return false;
      }}
      loading={error ? null : tuning ? `Tuning ${current ? `${current.number} ${current.name}` : channel}…` : holding ? "Buffering…" : null}
      notice={notice}
      error={error ? { title: `Can't play ${current?.name ?? channel}`, message: error, actions: <button className="btn-primary" onClick={() => setNonce((n) => n + 1)}>Try again</button> } : null}
      videoProps={{ onPlaying: () => setTuning(false), onProgress: () => videoRef.current?.buffered.length && setTuning(false) }}
      topActions={
        <>
          {recording && now?.recordingId && (
            <TopButton label="Watch this show from the beginning" onClick={() => go(`/recording/${now.recordingId}`, { replace: true })}>
              <SkipBack size={15} /> Start over
            </TopButton>
          )}
          {canRecord && now && now.recordingStatus !== "completed" && (
            <TopButton label={recording ? "Stop recording" : "Record this program"} onClick={() => void toggleRecord()} danger={recording} disabled={recBusy || now.recordingStatus === "scheduled"}>
              {recording ? <Square size={12} className="fill-current" /> : <CircleDot size={15} className="text-critical" />}
              {recording ? "Stop" : "Record"}
            </TopButton>
          )}
          <TopButton label="Channel up (Page Up)" onClick={() => zap(1)}>
            <ChevronUp size={18} />
          </TopButton>
          <TopButton label="Channel down (Page Down)" onClick={() => zap(-1)}>
            <ChevronDown size={18} />
          </TopButton>
          <TopButton label="Guide" onClick={exit}>
            <LayoutGrid size={17} />
          </TopButton>
        </>
      }
    />
  );
}

function LiveInfo({
  session,
  videoRef,
  hlsRef,
  channelName,
  speed,
}: {
  session: LiveSessionInfo | null;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  hlsRef: React.RefObject<HlsType | null>;
  channelName?: string;
  speed: () => number | null;
}) {
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);
  const v = videoRef.current;
  const q = v?.getVideoPlaybackQuality?.();
  const behind = v && v.seekable.length ? v.seekable.end(v.seekable.length - 1) - v.currentTime : 0;
  const latency = hlsRef.current?.latency;
  const rate = speed();
  return (
    <InfoRows
      rows={[
        ["Channel", session ? `${session.channel} ${channelName ?? session.name}` : "–"],
        ["Stream", session ? `H.264 ${session.height}p · deinterlaced` : "–"],
        ["Transcoder", session?.hw && session.hw !== "none" ? session.hw.toUpperCase() : "CPU (software)"],
        ["Playing", v && v.videoWidth ? `${v.videoWidth}×${v.videoHeight}` : "–"],
        ["Behind live", `${Math.max(0, behind).toFixed(0)}s${latency ? ` (latency ${latency.toFixed(1)}s)` : ""}`],
        ["Buffered", v ? `${bufferedAhead(v).toFixed(0)}s ahead` : "–"],
        ["Encoder speed", rate == null ? "Measuring…" : `${rate.toFixed(2)}× real time${rate < 0.95 ? " (can't keep up)" : ""}`],
        ["Dropped frames", q ? `${q.droppedVideoFrames} of ${q.totalVideoFrames}` : "–"],
      ]}
    />
  );
}
