export function fmtAgo(unixSec: number | null | undefined): string {
  if (!unixSec) return "never";
  const s = Math.max(0, Math.round(Date.now() / 1000 - unixSec));
  if (s < 45) return "just now";
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  if (s < 86400) return `${Math.round(s / 3600)}h ago`;
  if (s < 86400 * 30) return `${Math.round(s / 86400)}d ago`;
  return new Date(unixSec * 1000).toLocaleDateString();
}

/** 5400 → "1h 30m"; 754 → "12m". */
export function fmtRuntime(sec: number | null | undefined): string {
  if (!sec) return "";
  if (sec < 60) return `${Math.round(sec)}s`;
  const m = Math.round(sec / 60);
  if (m < 60) return `${m}m`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

/** Player-style clock: 754 → "12:34"; 3754 → "1:02:34". */
export function fmtClock(sec: number): string {
  const s = Math.floor(sec % 60);
  const m = Math.floor((sec / 60) % 60);
  const h = Math.floor(sec / 3600);
  const pad = (n: number) => String(n).padStart(2, "0");
  return h ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

export function fmtBytes(n: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i > 1 ? 1 : 0)} ${units[i]}`;
}

export function fmtResolution(w: number | null, h: number | null): string {
  if (!w || !h) return "";
  if (w >= 3200 || h >= 2000) return "4K";
  if (w >= 1800 || h >= 1000) return "1080p";
  if (w >= 1200 || h >= 700) return "720p";
  if (h >= 560) return "576p";
  return "SD";
}

/** Stable pseudo-random angle per id for placeholder gradients. */
export const placeholderAngle = (id: number) => `${(id * 47) % 360}deg`;

/** "2024-11-10 03:30" → "Nov 10, 2024 · 3:30 AM" (time only when present). */
export function fmtAirDate(s: string): string {
  const [d, t] = s.split(" ");
  const [y, m, day] = d.split("-").map(Number);
  if (!y || !m || !day) return s;
  const [hh, mm] = (t ?? "").split(":").map(Number);
  const date = new Date(y, m - 1, day, hh || 0, mm || 0);
  const ds = date.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  return t ? `${ds} · ${date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })}` : ds;
}

/** Unix seconds → "9:30 PM". */
export const fmtTime = (t: number) => new Date(t * 1000).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });

/** Unix seconds → "Today" / "Tomorrow" / "Wed, Oct 1". */
export function fmtDay(t: number): string {
  const d = new Date(t * 1000);
  const today = new Date();
  const days = Math.round((new Date(d.toDateString()).getTime() - new Date(today.toDateString()).getTime()) / 86400000);
  if (days === 0) return "Today";
  if (days === 1) return "Tomorrow";
  if (days === -1) return "Yesterday";
  return d.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
}

/** "Today 9:00 – 10:00 PM" */
export const fmtSlot = (start: number, end: number) => `${fmtDay(start)} ${fmtTime(start)} – ${fmtTime(end)}`;
