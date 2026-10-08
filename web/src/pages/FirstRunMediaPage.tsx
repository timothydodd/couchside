import { useState } from "react";
import { Check } from "lucide-react";
import AuthShell from "../components/auth/AuthShell";
import MediaLocations from "../components/settings/MediaLocations";
import { ErrorNote, Spinner } from "../components/ui";
import { api, useApi } from "../lib/api";
import { errText } from "../lib/errors";
import type { MediaLocations as Data } from "../lib/types";

/**
 * First run, step 2 of 2 (admins): where the media is. Drives, folders and
 * network shares, the same list as Settings → Server. Finishing marks setup
 * done and goes on to add the first library.
 */
export default function FirstRunMediaPage() {
  const { data } = useApi<Data>("/api/media/locations");
  const [count, setCount] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const n = count ?? data?.locations.length ?? 0;

  const finish = async (to: string) => {
    setBusy(true);
    setErr(null);
    try {
      await api("/api/setup/complete", { method: "POST" });
      location.assign(to);
    } catch (e) {
      setErr(errText(e));
      setBusy(false);
    }
  };

  return (
    <AuthShell title="Where's your media?" subtitle="Add the drives, folders or network shares your movies and TV shows are on. You can change these later in Settings → Server.">
      <div className="card mt-8 flex w-full max-w-2xl flex-col gap-4 p-6">
        <div className="text-xs font-semibold uppercase tracking-wide text-content-muted">Step 2 of 2</div>
        <MediaLocations onChange={setCount} />
        {err && <ErrorNote>{err}</ErrorNote>}
        <div className="flex flex-wrap items-center gap-3 border-t border-border-light pt-4">
          <button type="button" className="btn-primary" disabled={busy || n === 0} onClick={() => void finish("/libraries")}>
            {busy ? <Spinner size={14} /> : <Check size={15} />} Finish and add a library
          </button>
          <button type="button" className="btn-quiet" disabled={busy} onClick={() => void finish("/")}>
            Skip for now
          </button>
        </div>
      </div>
    </AuthShell>
  );
}
