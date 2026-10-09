import { FileArchive } from "lucide-react";

/** Settings → System → Advanced: a zip of what a bug report needs. */
export default function Diagnostics() {
  return (
    <section className="card p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="card-title">Diagnostics</div>
          <div className="text-xs text-content-muted">
            Versions, settings, encoder detection, library counts, database health, free disk space, failed jobs and the recent log, to
            attach to a bug report. API keys are masked and there are no passwords or sessions in it, but it names your folders and
            some file names: look it over before posting it anywhere public.
          </div>
        </div>
        <a className="btn-ghost shrink-0" href="/api/system/diagnostics" download>
          <FileArchive size={15} /> Download
        </a>
      </div>
    </section>
  );
}
