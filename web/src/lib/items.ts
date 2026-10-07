import type { EpisodeRow, ItemDetail, MediaFile } from "./types";

/** The files that make up a movie: its parts in order (the best copy of each), or its best copy (`chosen` when the profile picked one). */
export function featureFiles(files: MediaFile[], chosen?: MediaFile): MediaFile[] {
  const parts = files.filter((f) => f.role === "part" && !f.problem); // server order: part number, then biggest
  if (parts.length) return parts.filter((f, i) => i === 0 || parts[i - 1].partNo !== f.partNo);
  const copy = chosen ?? files.find((f) => f.role === "copy");
  return copy ? [copy] : [];
}

/** First episode that isn't finished: resume it, or start the next one. */
export function nextEpisode(seasons: NonNullable<ItemDetail["seasons"]>): EpisodeRow | undefined {
  const all = seasons.flatMap((s) => s.episodes).filter((e) => !e.problem);
  return all.find((e) => !e.watched) ?? all[0];
}

/** What Play does on a title's page: the file to play and where to resume (0 = from the start). */
export function playTarget(d: ItemDetail): { fileId: number; resumeAt: number } | null {
  if (d.item.kind === "series") {
    const next = nextEpisode(d.seasons ?? []);
    return next ? { fileId: next.fileId, resumeAt: next.positionSec } : null;
  }
  const versions = d.files.filter((f) => f.role === "copy" && !f.problem);
  const feature = featureFiles(d.files, versions.find((f) => f.id === d.versionFileId));
  // A split movie resumes in its first unfinished part, at a time on the whole movie's clock.
  const at = feature.findIndex((f) => !f.watched);
  const file = feature[Math.max(0, at)];
  if (!file) return null;
  const offset = feature.slice(0, Math.max(0, at)).reduce((t, f) => t + (f.durationSec ?? 0), 0);
  return { fileId: file.id, resumeAt: at >= 0 ? offset + file.positionSec : 0 };
}
