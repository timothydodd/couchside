// Minimal WebVTT parsing for subtitles loaded in chunks.

export interface Cue {
  start: number;
  end: number;
  text: string;
}

function parseTime(s: string): number {
  const parts = s.trim().split(":").map(Number);
  if (parts.some((n) => Number.isNaN(n))) return NaN;
  return parts.reduce((acc, n) => acc * 60 + n, 0);
}

export function parseVtt(body: string): Cue[] {
  const out: Cue[] = [];
  for (const block of body.replace(/\r/g, "").split(/\n\n+/)) {
    const lines = block.split("\n");
    const i = lines.findIndex((l) => l.includes("-->"));
    if (i < 0) continue;
    const [a, b] = lines[i].split("-->");
    const start = parseTime(a);
    const end = parseTime(b.trim().split(/\s+/)[0]);
    const text = lines.slice(i + 1).join("\n").trim();
    if (!Number.isNaN(start) && !Number.isNaN(end) && text) out.push({ start, end, text });
  }
  return out;
}
