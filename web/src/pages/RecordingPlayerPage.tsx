import { useEffect, useRef, useState } from "react";
import type HlsType from "hls.js";
import PlayerFrame from "../components/player/PlayerFrame";
import { InfoRows, type SettingSection } from "../components/player/SettingsMenu";
import { ApiError, api } from "../lib/api";
import { fmtTime } from "../lib/format";
import { nativeHls } from "../lib/playback";
import type { Recording } from "../lib/types";
import { useProfile } from "../stores/profile";
import { useRouter } from "../stores/router";

let hlsModule: Promise<typeof HlsType> | null = null;
const loadHls = () => (hlsModule ??= import("hls.js/light").then((m) => m.default));

interface Session {
  sessionId: string;
  playlist: string;
  height: number;
  hw: string;
  recording: Recording;
}

/**
 * A recording that's still being written. The timeline spans the whole
 * program; the recorded part is playable, and "Live" jumps to the recorded
 * edge of the same stream (no second tuner).
 */
export default function RecordingPlayerPage({ id }: { id: number }) {
  const { back } = useRouter();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<HlsType | null>(null);
  const [height, setHeight] = useState<number>(() => useProfile.getState().current?.prefs.liveHeight ?? 720);
  const [session, setSession] = useState<Session | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(true);
  const resumeAt = useRef(0);

  const exit = () => back("/livetv/recordings");

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    let cancelled = false;
    let hls: HlsType | null = null;
    let sid: string | null = null;
    setError(null);
    setStarting(true);
    const leave = () => {
      if (sid) void fetch(`/api/live/${sid}`, { method: "DELETE", keepalive: true });
    };
    window.addEventListener("pagehide", leave);
    (async () => {
      let s: Session;
      try {
        s = await api<Session>(`/api/dvr/recordings/${id}/watch`, { method: "POST", json: { height } });
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof ApiError ? e.message : String(e));
          setStarting(false);
        }
        return;
      }
      sid = s.sessionId;
      if (cancelled) return leave();
      setSession(s);
      const Hls = await loadHls().catch(() => null);
      if (cancelled) return;
      if (!Hls || !Hls.isSupported()) {
        if (!nativeHls()) return setError("This browser can't play streams.");
        v.src = s.playlist;
        v.addEventListener("loadedmetadata", () => (v.currentTime = resumeAt.current), { once: true });
        void v.play().catch(() => {});
        return;
      }
      const player = new Hls({ startPosition: resumeAt.current, maxBufferLength: 30, backBufferLength: 3600 });
      hls = player;
      hlsRef.current = player;
      player.on(Hls.Events.ERROR, (_e, data) => {
        if (!data.fatal) return;
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) return player.recoverMediaError();
        setError(`Playback stopped (${data.details}).`);
      });
      player.loadSource(s.playlist);
      player.attachMedia(v);
      player.once(Hls.Events.MANIFEST_PARSED, () => void v.play().catch(() => {}));
    })();
    return () => {
      cancelled = true;
      resumeAt.current = v.currentTime || resumeAt.current;
      window.removeEventListener("pagehide", leave);
      hls?.destroy();
      hlsRef.current = null;
      leave();
      v.removeAttribute("src");
      v.load();
    };
  }, [id, height]);

  const rec = session?.recording;
  const startAt = rec ? rec.startedAt ?? rec.startAt - rec.padBefore : 0;
  const endAt = rec ? rec.endAt + rec.padAfter : 0;

  const settings: SettingSection[] = [
    {
      id: "quality",
      label: "Quality",
      value: `${height}p`,
      options: [1080, 720, 480].map((h) => ({ id: String(h), label: `${h}p`, detail: h === 1080 ? "8 Mbps" : h === 720 ? "4 Mbps" : "1.5 Mbps", active: h === height })),
      onSelect: (idv) => setHeight(Number(idv)),
    },
    {
      id: "info",
      label: "Playback info",
      value: session ? `${session.height}p` : "",
      content: (
        <InfoRows
          rows={[
            ["Recording", rec ? `${rec.channel} ${rec.channelName}, ${fmtTime(startAt)} – ${fmtTime(endAt)}` : "–"],
            ["Stream", session ? `H.264 ${session.height}p, following the file as it's written` : "–"],
            ["Transcoder", session?.hw && session.hw !== "none" ? session.hw.toUpperCase() : "CPU (software)"],
          ]}
        />
      ),
    },
  ];

  return (
    <PlayerFrame
      videoRef={videoRef}
      badge={
        <>
          <span className="inline-flex items-center gap-1 rounded bg-critical px-1.5 py-px text-[10px] font-bold tracking-wide text-white">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-white" /> RECORDING
          </span>
          {rec && <span>{rec.channel} {rec.channelName}</span>}
        </>
      }
      title={rec?.title ?? " "}
      subtitle={rec && [rec.episodeNum, rec.episodeTitle].filter(Boolean).join(" · ")}
      timeline={rec ? { kind: "recording", startAt, endAt } : { kind: "live" }}
      liveEdge={() => hlsRef.current?.liveSyncPosition ?? null}
      settings={settings}
      onBack={exit}
      loading={starting && !error ? "Starting from the beginning…" : null}
      error={error ? { title: "Can't play this recording", message: error } : null}
      videoProps={{ onPlaying: () => setStarting(false), onEnded: exit }}
    />
  );
}
