import { useCallback, useEffect, useRef, useState } from "react";
import type HlsType from "hls.js";
import { ChevronDown, ChevronUp, CircleDot, LayoutGrid, SkipBack, Square } from "lucide-react";
import PlayerFrame, { TopButton } from "../components/player/PlayerFrame";
import { InfoRows, type SettingSection } from "../components/player/SettingsMenu";
import { ApiError, api, useApi } from "../lib/api";
import { fmtTime } from "../lib/format";
import { nativeHls } from "../lib/playback";
import type { ChannelNow, LiveSessionInfo, Program } from "../lib/types";
import { useRouter } from "../stores/router";

const LIVE_QUALITIES = [1080, 720, 480] as const;
let hlsModule: Promise<typeof HlsType> | null = null;
const loadHls = () => (hlsModule ??= import("hls.js/light").then((m) => m.default));

/** Live TV: the server tunes and transcodes; you can pause and rewind within the session. */
export default function LivePlayerPage({ channel }: { channel: string }) {
  const { go } = useRouter();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<HlsType | null>(null);
  const { data: channels, reload: reloadChannels } = useApi<ChannelNow[]>("/api/livetv/channels", { pollMs: 60000 });
  const [height, setHeight] = useState<number>(() => Number(localStorage.getItem("couchside:live-height")) || 720);
  const [session, setSession] = useState<LiveSessionInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tuning, setTuning] = useState(true);
  const [recBusy, setRecBusy] = useState(false);
  const [nonce, setNonce] = useState(0);

  const playable = (channels ?? []).filter((c) => !c.drm);
  const idx = playable.findIndex((c) => c.number === channel);
  const current = channels?.find((c) => c.number === channel);
  const now: Program | null | undefined = current?.now ?? session?.now;

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
        liveSyncDurationCount: 3,
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
      player.loadSource(s.playlist);
      player.attachMedia(v);
      player.once(Hls.Events.MANIFEST_PARSED, () => void v.play().catch(() => {}));
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
        try {
          localStorage.setItem("couchside:live-height", String(h));
        } catch {
          /* ignore */
        }
        setHeight(h);
      },
    },
    { id: "info", label: "Playback info", value: session ? `${session.height}p` : "", content: <LiveInfo session={session} videoRef={videoRef} hlsRef={hlsRef} channelName={current?.name} /> },
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
      timeline={{ kind: "live" }}
      liveEdge={() => hlsRef.current?.liveSyncPosition ?? null}
      settings={settings}
      onBack={exit}
      onKey={(e) => {
        if (e.key === "PageUp" || (e.key === "ArrowUp" && e.ctrlKey)) return zap(1), true;
        if (e.key === "PageDown" || (e.key === "ArrowDown" && e.ctrlKey)) return zap(-1), true;
        return false;
      }}
      loading={tuning && !error ? `Tuning ${current ? `${current.number} ${current.name}` : channel}…` : null}
      error={error ? { title: `Can't play ${current?.name ?? channel}`, message: error, actions: <button className="btn-primary" onClick={() => setNonce((n) => n + 1)}>Try again</button> } : null}
      videoProps={{ onPlaying: () => setTuning(false) }}
      topActions={
        <>
          {recording && now?.recordingId && (
            <TopButton label="Watch this show from the beginning" onClick={() => go(`/recording/${now.recordingId}`, { replace: true })}>
              <SkipBack size={15} /> Start over
            </TopButton>
          )}
          {now && now.recordingStatus !== "completed" && (
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
}: {
  session: LiveSessionInfo | null;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  hlsRef: React.RefObject<HlsType | null>;
  channelName?: string;
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
  return (
    <InfoRows
      rows={[
        ["Channel", session ? `${session.channel} ${channelName ?? session.name}` : "–"],
        ["Stream", session ? `H.264 ${session.height}p · deinterlaced` : "–"],
        ["Transcoder", session?.hw && session.hw !== "none" ? session.hw.toUpperCase() : "CPU (software)"],
        ["Playing", v && v.videoWidth ? `${v.videoWidth}×${v.videoHeight}` : "–"],
        ["Behind live", `${Math.max(0, behind).toFixed(0)}s${latency ? ` (latency ${latency.toFixed(1)}s)` : ""}`],
        ["Dropped frames", q ? `${q.droppedVideoFrames} of ${q.totalVideoFrames}` : "–"],
      ]}
    />
  );
}
