// Labels for audio and subtitle tracks in the player's settings menu.

export interface AudioTrack {
  index: number;
  codec: string;
  channels: number;
  language: string;
  title: string;
  default: boolean;
}

export interface SubtitleTrack {
  key: string;
  language: string;
  title: string;
  codec: string;
  text: boolean;
  forced: boolean;
  default: boolean;
  external: boolean;
  index: number;
}

let names: Intl.DisplayNames | null = null;
try {
  names = new Intl.DisplayNames(undefined, { type: "language" });
} catch {
  names = null;
}

export function languageName(code: string): string {
  if (!code || code === "und") return "Unknown";
  try {
    return names?.of(code) ?? code;
  } catch {
    return code;
  }
}

const CODEC: Record<string, string> = {
  aac: "AAC", ac3: "Dolby Digital", eac3: "Dolby Digital+", dts: "DTS", truehd: "Dolby TrueHD", flac: "FLAC",
  mp3: "MP3", opus: "Opus", vorbis: "Vorbis", pcm_s16le: "PCM", pcm_s24le: "PCM",
};

export function channelsLabel(n: number): string {
  if (n === 1) return "Mono";
  if (n === 2) return "Stereo";
  if (n === 6) return "5.1";
  if (n === 8) return "7.1";
  return n ? `${n} ch` : "";
}

export function audioLabel(a: AudioTrack): string {
  return [languageName(a.language), CODEC[a.codec] ?? a.codec.toUpperCase(), channelsLabel(a.channels)].filter(Boolean).join(" · ");
}

export function subtitleLabel(s: SubtitleTrack): string {
  const parts = [languageName(s.language)];
  if (s.title && s.title.toLowerCase() !== s.language) parts.push(s.title);
  if (s.forced) parts.push("Forced");
  return parts.join(" · ");
}

export function subtitleDetail(s: SubtitleTrack): string {
  if (!s.text) return "Picture subtitles: the server draws them into the video";
  return s.external ? `External ${s.codec.toUpperCase()} file` : `${s.codec.toUpperCase()} text`;
}
