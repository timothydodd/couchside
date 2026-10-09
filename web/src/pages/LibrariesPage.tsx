import { useState } from "react";
import { ChevronDown, Clapperboard, Folder, FolderPlus, RefreshCw, ScanSearch, Settings2, Trash2, Tv } from "lucide-react";
import Link from "../components/Link";
import LibraryForm from "../components/LibraryForm";
import { EmptyState, ErrorNote, Loading, MenuButton, PageHeader } from "../components/ui";
import { api, useApi } from "../lib/api";
import { fmtAgo } from "../lib/format";
import type { Library } from "../lib/types";
import { useStatus } from "../stores/status";
import { attempt, notify } from "../lib/notices";
import { confirmDialog } from "../lib/ask";
import { useTitle } from "../lib/title";

export default function LibrariesPage() {
  const { data, error, reload } = useApi<Library[]>("/api/libraries", { pollMs: 5000 });
  useTitle("Libraries");
  const [adding, setAdding] = useState(false);

  const scan = attempt("Couldn't start the scan", async (id: number) => {
    await api(`/api/libraries/${id}/scan`, { method: "POST" });
    void useStatus.getState().refresh();
  });
  const rematch = attempt("Couldn't re-match the library", async (l: Library) => {
    if (!(await confirmDialog({ title: `Look up every title in "${l.name}" again?`, body: "This refreshes details, posters, backdrops and cast from the metadata providers. Matches you fixed by hand stay as they are.", action: "Re-match" }))) return;
    const r = await api<{ queued: number }>(`/api/libraries/${l.id}/rematch`, { method: "POST" });
    notify(`Queued ${r.queued} lookup${r.queued === 1 ? "" : "s"}; progress is on the Activity page.`, "info");
    void useStatus.getState().refresh();
  });
  const remove = attempt("Couldn't remove the library", async (l: Library) => {
    if (!(await confirmDialog({ title: `Remove "${l.name}" from Couchside?`, body: "Your files are not touched; only the library entry, watch history and artwork go.", action: "Remove", danger: true }))) return;
    await api(`/api/libraries/${l.id}`, { method: "DELETE" });
    await reload();
    void useStatus.getState().refresh();
  });

  return (
    <div>
      <PageHeader title="Libraries" subtitle="Folders Couchside scans for movies and TV shows">
        {!adding && (
          <button className="btn-primary" onClick={() => setAdding(true)}>
            <FolderPlus size={15} /> Add library
          </button>
        )}
      </PageHeader>
      <div className="flex flex-col gap-4 gutter py-5">
        {error && <ErrorNote>{error}</ErrorNote>}
        {!data && !error && <Loading />}
        {adding && (
          <LibraryForm
            onDone={() => {
              setAdding(false);
              void reload();
              void useStatus.getState().refresh();
            }}
            onCancel={() => setAdding(false)}
          />
        )}
        {data?.length === 0 && !adding && (
          <div className="card">
            <EmptyState icon={<Folder size={36} strokeWidth={1.5} />} title="No libraries yet">
              Add a folder of movies or TV shows to get started.
            </EmptyState>
          </div>
        )}
        {data?.map((l) => (
            <div key={l.id} className="card flex flex-wrap items-center gap-4 px-4 py-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-md bg-muted text-accent">
                {l.kind === "movies" ? <Clapperboard size={18} /> : <Tv size={18} />}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold text-content">{l.name}</span>
                  <span className="tint-muted badge">{l.kind === "movies" ? "Movies" : "TV shows"}</span>
                </div>
                <div className="mono mt-0.5 truncate text-content-muted">{l.path}</div>
              </div>
              <div className="flex w-full items-center justify-between gap-3 sm:contents">
              <div className="text-xs text-content-secondary sm:text-right">
                <div className="tabular-nums">
                  {l.itemCount.toLocaleString()} {l.kind === "movies" ? "movies" : "shows"} · {l.fileCount.toLocaleString()} files
                </div>
                <div className="text-content-muted">Scanned {fmtAgo(l.lastScanAt)}</div>
              </div>
              <div className="split-btn sm:ml-auto">
                <Link to={`/libraries/${l.id}`} className="btn-ghost" title="Name, folder and options, and every title's quality and duplicates">
                  <Settings2 size={15} /> Settings
                </Link>
                <MenuButton
                  label={`More for ${l.name}`}
                  icon={<ChevronDown size={15} />}
                  align="end"
                  className="btn-ghost split-btn-menu"
                  items={[
                    { id: "scan", label: "Scan", detail: "Look for new, changed and removed files.", icon: <ScanSearch size={15} />, onSelect: () => void scan(l.id) },
                    {
                      id: "rematch",
                      label: "Re-match",
                      detail: "Look up details, artwork and cast again for every title.",
                      icon: <RefreshCw size={15} />,
                      onSelect: () => void rematch(l),
                    },
                    {
                      id: "delete",
                      label: "Remove library",
                      detail: "Files on disk stay; its watch history and artwork go.",
                      icon: <Trash2 size={15} />,
                      danger: true,
                      onSelect: () => void remove(l),
                    },
                  ]}
                />
              </div>
              </div>
            </div>
        ))}
      </div>
    </div>
  );
}
