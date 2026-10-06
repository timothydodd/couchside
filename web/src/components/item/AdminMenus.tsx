import { useState } from "react";
import { Cpu, Eye, EyeOff, FastForward, FolderOpen, MoreHorizontal, Pencil, RefreshCw, RotateCcw, Scissors, Trash2 } from "lucide-react";
import EditDetails from "./EditDetails";
import { MenuButton, type MenuItem } from "../ui";
import { api } from "../../lib/api";
import { attempt, notify } from "../../lib/notices";
import type { DeleteResult, EpisodeRow, Item } from "../../lib/types";
import { useRouter } from "../../stores/router";
import { useStatus } from "../../stores/status";

const episodeName = (e: EpisodeRow) =>
  `${e.airDate ? e.airDate : `S${e.season}·E${e.episode}`}${e.title ? ` "${e.title}"` : ""}`;

const queued = (what: string) => {
  notify(`${what}; progress is on the Activity page.`, "info");
  void useStatus.getState().refresh();
};

/** The "⋯" on an episode row, for admins: re-scan, encode, commercials, delete. */
export function EpisodeActions({ item, e, onChange }: { item: Item; e: EpisodeRow; onChange: () => void }) {
  const comskip = useStatus((s) => s.status?.comskip);
  const go = useRouter((s) => s.go);
  const file = `/api/files/${e.fileId}`;
  const readable = !e.problem;

  const items: MenuItem[] = [
    {
      id: "watched",
      label: e.watched ? "Mark unwatched" : "Mark watched",
      icon: e.watched ? <EyeOff size={15} /> : <Eye size={15} />,
      onSelect: attempt("Couldn't change watched", async () => {
        await api(`${file}/watched`, { method: "POST", json: { watched: !e.watched } });
        onChange();
      }),
    },
    {
      id: "rescan",
      label: "Re-scan file",
      detail: "Reads it again: length, codecs, episode number and a new still.",
      icon: <RefreshCw size={15} />,
      onSelect: attempt("Couldn't re-scan", async () => {
        await api(`${file}/rescan`, { method: "POST" });
        notify(`Re-scanned ${episodeName(e)}.`, "info");
        onChange();
      }),
    },
  ];
  if (readable) {
    items.push({
      id: "optimize",
      label: "Optimize",
      detail: "Encode a browser-friendly H.264 copy of this episode.",
      icon: <Cpu size={15} />,
      onSelect: attempt("Couldn't queue the encode", async () => {
        await api(`${file}/optimize`, { method: "POST" });
        queued(`Encoding ${episodeName(e)}`);
      }),
    });
    if (comskip)
      items.push({
        id: "commercials",
        label: "Find commercials",
        detail: "Runs detection on this episode, replacing any breaks found before.",
        icon: <Scissors size={15} />,
        onSelect: attempt("Couldn't start detection", async () => {
          await api(`${file}/commercials`, { method: "POST" });
          queued(`Looking for commercials in ${episodeName(e)}`);
        }),
      });
  }
  items.push({
    id: "delete",
    label: "Delete from disk",
    detail: "Removes the video file and its subtitles. This can't be undone.",
    icon: <Trash2 size={15} />,
    danger: true,
    onSelect: attempt("Couldn't delete", async () => {
      if (!confirm(`Delete ${episodeName(e)} of ${item.title} from disk? This can't be undone.`)) return;
      const r = await api<DeleteResult>(file, { method: "DELETE" });
      notify(`Deleted ${episodeName(e)}.`, "info");
      if (r.itemsRemoved.includes(item.id)) go("/tv", { replace: true });
      else onChange();
    }),
  });

  return <MenuButton label="Episode actions" icon={<MoreHorizontal size={16} />} items={items} align="end" className="btn-quiet !p-2" />;
}

/** The "⋯" beside Mark watched, for admins: the whole movie or show. */
export function TitleActions({ item, onChange }: { item: Item; onChange: () => void }) {
  const comskip = useStatus((s) => s.status?.comskip);
  const go = useRouter((s) => s.go);
  const [editing, setEditing] = useState(false);
  const series = item.kind === "series";
  const what = series ? "episode" : "file";
  const count = (n: number) => `${n} ${what}${n === 1 ? "" : "s"}`;

  const items: MenuItem[] = [
    {
      id: "edit",
      label: "Edit details",
      detail: "Title, year, rating, genres and description, by hand.",
      icon: <Pencil size={15} />,
      onSelect: () => setEditing(true),
    },
    {
      id: "scan",
      label: "Scan library",
      detail: "Looks for new, changed and removed files in this title's library.",
      icon: <RefreshCw size={15} />,
      onSelect: attempt("Couldn't start the scan", async () => {
        await api(`/api/libraries/${item.libraryId}/scan`, { method: "POST" });
        queued("Scanning the library");
      }),
    },
    {
      id: "optimize",
      label: series ? "Optimize every episode" : "Optimize",
      detail: "Encode browser-friendly copies of files that can't play directly.",
      icon: <Cpu size={15} />,
      onSelect: attempt("Couldn't queue the encodes", async () => {
        const r = await api<{ queued: number }>(`/api/items/${item.id}/optimize`, { method: "POST" });
        if (r.queued) queued(`Encoding ${count(r.queued)}`);
        else notify(series ? "Every episode already plays in the browser or has an optimized copy." : "This already plays in the browser or has an optimized copy.", "info");
      }),
    },
  ];
  if (series)
    items.push({
      id: "intros",
      label: "Find intros",
      detail: "Compares each season's episodes to find the opening they share.",
      icon: <FastForward size={15} />,
      onSelect: attempt("Couldn't start looking for intros", async () => {
        await api(`/api/items/${item.id}/intros`, { method: "POST" });
        queued("Looking for intros");
      }),
    });
  if (comskip) {
    const find = (redo: boolean) =>
      attempt("Couldn't start detection", async () => {
        const r = await api<{ queued: number }>(`/api/items/${item.id}/commercials`, { method: "POST", json: { redo } });
        if (r.queued) queued(`Looking for commercials in ${count(r.queued)}`);
        else notify(series ? "Every episode has already been checked for commercials." : "This has already been checked for commercials.", "info");
      });
    items.push(
      {
        id: "commercials",
        label: "Find commercials",
        detail: series ? "In episodes that haven't been checked yet." : "If it hasn't been checked yet.",
        icon: <Scissors size={15} />,
        onSelect: find(false),
      },
      {
        id: "commercials-redo",
        label: series ? "Check every episode again" : "Check for commercials again",
        detail: "Replaces the breaks found before.",
        icon: <RotateCcw size={15} />,
        onSelect: find(true),
      },
    );
  }
  items.push(
    {
      id: "manage",
      label: "Manage in library",
      detail: "Files, duplicates, artwork and matching, in the library's table.",
      icon: <FolderOpen size={15} />,
      onSelect: () => go(`/libraries/${item.libraryId}`),
    },
    {
      id: "delete",
      label: series ? "Delete show from disk" : "Delete from disk",
      detail: `Removes ${series ? `all ${count(item.fileCount)}` : "the video files"} and their subtitles. This can't be undone.`,
      icon: <Trash2 size={15} />,
      danger: true,
      onSelect: attempt("Couldn't delete", async () => {
        if (!confirm(`Delete ${item.title} (${count(item.fileCount)}) from disk? This can't be undone.`)) return;
        await api<DeleteResult>(`/api/items/${item.id}`, { method: "DELETE" });
        notify(`Deleted ${item.title}.`, "info");
        go(series ? "/tv" : "/movies", { replace: true });
        onChange();
      }),
    },
  );

  return (
    <>
      <MenuButton label="More actions" icon={<MoreHorizontal size={16} />} items={items} align="end" className="btn-ghost !px-2.5 !py-2" />
      {editing && <EditDetails item={item} onClose={() => setEditing(false)} onSaved={onChange} />}
    </>
  );
}
