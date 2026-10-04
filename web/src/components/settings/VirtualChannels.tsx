import { useRef, useState } from "react";
import { Download, MoreHorizontal, Pencil, Plus, RadioTower, Trash2, Upload } from "lucide-react";
import ChannelEditor from "./ChannelEditor";
import Link from "../Link";
import { ErrorNote, MenuButton } from "../ui";
import { api, useApi } from "../../lib/api";
import type { VirtualChannel, VirtualConfig } from "../../lib/types";
import { useStatus } from "../../stores/status";
import { attempt, notify } from "../../lib/notices";

/**
 * Couchside's own channels: made from the library, they play around the clock
 * in Live TV and the guide, with or without a tuner. Admin only.
 */
export default function VirtualChannels() {
  const { data, error, reload } = useApi<VirtualChannel[]>("/api/livetv/virtual");
  const [editing, setEditing] = useState<VirtualChannel | "new" | null>(null);
  const saved = () => {
    void reload();
    void useStatus.getState().refresh(); // Live TV may have just appeared
  };
  const remove = attempt("Couldn't delete the channel", async (c: VirtualChannel) => {
    if (!confirm(`Delete channel ${c.number} ${c.name}? Your files aren't touched.`)) return;
    await api(`/api/livetv/virtual/${c.id}`, { method: "DELETE" });
    saved();
  });

  // Import a file made by Export (here or on another server). Channels whose
  // number is already one of yours are replaced.
  const picker = useRef<HTMLInputElement>(null);
  const importFile = attempt("Couldn't import the channels", async (file: File) => {
    let json: unknown;
    try {
      json = JSON.parse(await file.text());
    } catch {
      throw new Error("that file isn't JSON");
    }
    const r = await api<{ created: number; updated: number; skipped: string[]; warnings: string[] }>("/api/livetv/virtual/import", { method: "POST", json });
    saved();
    const count = (n: number, what: string) => (n ? `${n} ${what}` : "");
    notify([count(r.created, "added"), count(r.updated, "replaced")].filter(Boolean).join(", ") || "No channels were imported.", "info");
    for (const line of r.skipped) notify(`Left out ${line}`);
    for (const line of r.warnings) notify(line);
  });

  return (
    <section className="card p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div>
          <div className="card-title">Your channels</div>
          <div className="text-xs text-content-muted">Channels made from your library that play around the clock, like broadcast TV. No tuner needed.</div>
        </div>
        <div className="flex shrink-0 flex-wrap justify-end gap-2">
          <input
            ref={picker}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              e.target.value = ""; // so picking the same file again still fires
              if (f) void importFile(f);
            }}
          />
          <button className="btn-ghost" onClick={() => picker.current?.click()} title="Add channels from a file exported here or on another Couchside">
            <Upload size={15} /> Import
          </button>
          {data && data.length > 0 && (
            <a className="btn-ghost" href="/api/livetv/virtual/export" download title="Save these channels as a file, to import on another Couchside">
              <Download size={15} /> Export
            </a>
          )}
          <button className="btn-primary" onClick={() => setEditing("new")}>
            <Plus size={15} /> New channel
          </button>
        </div>
      </div>
      {error && !data && <ErrorNote>Couldn't load your channels: {error}</ErrorNote>}
      {data && data.length === 0 && (
        <p className="text-sm text-content-muted">
          None yet. Try a movie night, a sitcom marathon or a decade channel, with old commercials if you have a folder of them.
        </p>
      )}
      {data && data.length > 0 && (
        <ul className="divide-y divide-border-light rounded-md border border-border-light">
          {data.map((c) => (
            <li key={c.id} className="flex items-center gap-3 px-3 py-2">
              <RadioTower size={16} className="shrink-0 text-accent" />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm text-content">
                  <span className="mr-2 font-semibold tabular-nums">{c.number}</span>
                  {c.name}
                </div>
                <div className="truncate text-xs text-content-muted">{describe(c.config)}</div>
                {c.error && <div className="text-xs text-warning">{c.error}</div>}
              </div>
              <Link to={`/watch/${c.number}`} className="btn-quiet !text-xs">
                Watch
              </Link>
              <MenuButton
                label={`More for ${c.name}`}
                icon={<MoreHorizontal size={16} />}
                align="end"
                className="btn-quiet"
                items={[
                  { id: "edit", label: "Edit", icon: <Pencil size={15} />, onSelect: () => setEditing(c) },
                  { id: "delete", label: "Delete channel", icon: <Trash2 size={15} />, danger: true, onSelect: () => void remove(c) },
                ]}
              />
            </li>
          ))}
        </ul>
      )}
      {editing && <ChannelEditor channel={editing === "new" ? undefined : editing} onClose={() => setEditing(null)} onSaved={saved} />}
    </section>
  );
}

/** A one-line summary: "Shows · Comedy · 1990–1999 · in order · commercials". */
function describe(c: VirtualConfig): string {
  const parts: string[] = [];
  if (c.kinds?.length === 1) parts.push(c.kinds[0] === "movie" ? "Movies" : "Shows");
  if (c.items?.length) parts.push(`${c.items.length} picked ${c.items.length === 1 ? "title" : "titles"}`);
  if (c.genres?.length) parts.push(c.genres.join(", "));
  if (c.yearFrom || c.yearTo) parts.push(`${c.yearFrom || "…"}–${c.yearTo || "…"}`);
  if (c.minRating) parts.push(`${c.minRating}+`);
  parts.push(c.order === "sequential" ? "in order" : "shuffled");
  if (c.filler?.folder) parts.push("commercials");
  return parts.join(" · ");
}
