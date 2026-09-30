import type { RuleMode, RuleSummary } from "../../lib/types";

export const MODE_TEXT: Record<RuleMode, { label: string; detail: string }> = {
  missing: { label: "Episodes I don't have", detail: "Skips anything already in your library or recorded. Airings without episode info are recorded only when new." },
  new: { label: "New episodes only", detail: "First airings only; reruns are skipped." },
  all: { label: "Every airing", detail: "Records every airing, including repeats of the same episode." },
};

/** One sentence explaining what a rule run did. */
export function describeSummary(s: RuleSummary | null | undefined): string {
  if (!s) return "";
  const head =
    s.scheduled === 0
      ? `Nothing to record in the ${s.airings ? "current guide" : "guide right now (no upcoming airings)"}`
      : `Recording ${s.scheduled} airing${s.scheduled === 1 ? "" : "s"} in the current guide`;
  const skipped: string[] = [];
  if (s.alreadyHave) skipped.push(`${s.alreadyHave} already in your library`);
  if (s.alreadyRecorded) skipped.push(`${s.alreadyRecorded} already recorded or scheduled`);
  if (s.duplicates) skipped.push(`${s.duplicates} repeat airing${s.duplicates === 1 ? "" : "s"}`);
  if (s.notNew) skipped.push(`${s.notNew} rerun${s.notNew === 1 ? "" : "s"}`);
  if (s.noEpisodeInfo) skipped.push(`${s.noEpisodeInfo} rerun${s.noEpisodeInfo === 1 ? "" : "s"} with no episode info`);
  if (s.otherChannel) skipped.push(`${s.otherChannel} on other channels`);
  if (s.cancelled) skipped.push(`${s.cancelled} you cancelled`);
  if (s.locked) skipped.push(`${s.locked} on copy-protected channels`);
  let out = head + (skipped.length ? `. Skipped ${skipped.join(", ")}.` : ".");
  if (s.conflicts) out += ` ${s.conflicts} airing${s.conflicts === 1 ? "" : "s"} couldn't fit because every tuner is booked.`;
  return out;
}

export const KEEP_OPTIONS = [0, 3, 5, 10, 20];
export const keepLabel = (n: number) => (n ? `Keep last ${n}` : "Keep all");
