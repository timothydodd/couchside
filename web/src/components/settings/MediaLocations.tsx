import { useState } from "react";
import { CheckCircle2, FolderOpen, HardDrive, KeyRound, Network, Plus, Trash2, TriangleAlert } from "lucide-react";
import FolderPicker from "../FolderPicker";
import { ErrorNote, Segmented, Spinner } from "../ui";
import { api, useApi } from "../../lib/api";
import { errText } from "../../lib/errors";
import type { MediaLocation, MediaLocations as Data } from "../../lib/types";

/**
 * Where media can be: folders and drives on the server, and (Windows)
 * network shares with a user name and password. Libraries and the DVR folder
 * must be inside one. Used in Settings → Server and the first-run setup.
 */
export default function MediaLocations({ onChange }: { onChange?: (count: number) => void }) {
  const { data, reload } = useApi<Data>("/api/media/locations");
  const [adding, setAdding] = useState(false);
  if (!data) return null;
  const done = async (next?: Data) => {
    await reload();
    onChange?.((next ?? data).locations.length);
  };

  return (
    <div className="flex flex-col gap-3">
      {data.locations.length === 0 && !adding && (
        <p className="text-sm text-content-muted">No media locations yet. Add the drive, folder or network share your movies and TV shows are on.</p>
      )}
      {data.locations.map((l) => (
        <LocationRow key={l.id || l.path} l={l} shares={data.shares} envVar={data.envVar} onDone={done} />
      ))}
      {adding ? (
        <AddLocation
          shares={data.shares}
          onCancel={() => setAdding(false)}
          onAdded={async (next) => {
            setAdding(false);
            await done(next);
          }}
        />
      ) : (
        <div>
          <button type="button" className="btn-ghost" onClick={() => setAdding(true)}>
            <Plus size={14} /> Add a location
          </button>
        </div>
      )}
    </div>
  );
}

function LocationRow({ l, shares, envVar, onDone }: { l: MediaLocation; shares: boolean; envVar: string; onDone: (next?: Data) => Promise<void> }) {
  const [editing, setEditing] = useState(false);
  const [sure, setSure] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const remove = async () => {
    setBusy(true);
    setErr(null);
    try {
      await onDone(await api<Data>(`/api/media/locations/${l.id}`, { method: "DELETE" }));
    } catch (e) {
      setErr(errText(e));
      setSure(false);
    } finally {
      setBusy(false);
    }
  };
  const Icon = l.share ? Network : HardDrive;
  return (
    <div className="rounded-md border border-border-light px-3 py-2.5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <Icon size={16} className="shrink-0 text-accent" />
        <span className="mono min-w-0 flex-1 truncate text-sm text-content" title={l.path}>
          {l.path}
        </span>
        {l.fromEnv ? (
          <span className="badge tint-muted" title={`Set by ${envVar}; change it where the server is configured`}>
            From {envVar}
          </span>
        ) : (
          <div className="flex gap-1">
            {l.share && shares && (
              <button type="button" className="btn-quiet !px-2 !py-1 !text-xs" onClick={() => setEditing((e) => !e)} aria-expanded={editing}>
                <KeyRound size={13} /> Sign-in
              </button>
            )}
            {sure ? (
              <>
                <button type="button" className="btn-quiet !px-2 !py-1 !text-xs" onClick={() => setSure(false)}>
                  Keep
                </button>
                <button type="button" className="btn-quiet !px-2 !py-1 !text-xs text-critical" disabled={busy} onClick={() => void remove()}>
                  {busy ? <Spinner size={12} /> : <Trash2 size={13} />} Remove
                </button>
              </>
            ) : (
              <button type="button" className="btn-quiet !px-2 !py-1 !text-xs" onClick={() => setSure(true)} aria-label={`Remove ${l.path}`}>
                <Trash2 size={13} />
              </button>
            )}
          </div>
        )}
      </div>
      <p className={`mt-1 flex items-center gap-1.5 text-xs ${l.error || !l.found ? "text-warning" : "text-content-muted"}`}>
        {l.error ? (
          <>
            <TriangleAlert size={12} /> {l.error}
          </>
        ) : !l.found ? (
          <>
            <TriangleAlert size={12} /> Can't be opened right now{l.share ? " (is the NAS on, and does it need a sign-in?)" : ""}
          </>
        ) : (
          <>
            <CheckCircle2 size={12} className="text-good" /> Ready{l.user ? `, signed in as ${l.user}` : ""}
          </>
        )}
      </p>
      {err && <p className="mt-1 text-xs text-critical">{err}</p>}
      {editing && (
        <ShareSignIn
          user={l.user}
          hasPassword={l.hasPassword}
          submitLabel="Save and sign in"
          onCancel={() => setEditing(false)}
          onSubmit={async (user, password) => {
            const next = await api<Data>(`/api/media/locations/${l.id}`, { method: "PUT", json: { user, password } });
            setEditing(false);
            await onDone(next);
          }}
        />
      )}
    </div>
  );
}

function AddLocation({ shares, onCancel, onAdded }: { shares: boolean; onCancel: () => void; onAdded: (next: Data) => Promise<void> }) {
  const [kind, setKind] = useState<"local" | "share">("local");
  const [path, setPath] = useState("");
  const [browsing, setBrowsing] = useState(true);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const add = async (user = "", password: string | null = null) => {
    setBusy(true);
    setErr(null);
    try {
      await onAdded(await api<Data>("/api/media/locations", { method: "POST", json: { path, user, password } }));
    } catch (e) {
      setErr(errText(e));
      throw e;
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-md border border-accent/40 bg-raised p-3">
      {shares && (
        <Segmented
          label="Kind of location"
          value={kind}
          onChange={(k) => (setKind(k), setPath(""), setErr(null))}
          options={[
            { id: "local", label: "Folder or drive" },
            { id: "share", label: "Network share" },
          ]}
        />
      )}
      {kind === "local" ? (
        <>
          <div className="mt-3 flex gap-2">
            <input
              className="field mono min-w-0 flex-1"
              placeholder={shares ? "D:\\Media" : "/mnt/media"}
              value={path}
              onChange={(e) => setPath(e.target.value)}
              aria-label="Folder"
              autoFocus
            />
            <button type="button" className="btn-ghost" onClick={() => setBrowsing((b) => !b)} aria-expanded={browsing}>
              <FolderOpen size={14} /> Browse
            </button>
          </div>
          {browsing && <FolderPicker path={path} onPick={setPath} all />}
          {!shares && (
            <p className="mt-1.5 text-xs text-content-muted">For a NAS, mount the share on this machine (or in the container) and add the folder it's mounted at.</p>
          )}
          {err && (
            <div className="mt-2">
              <ErrorNote>{err}</ErrorNote>
            </div>
          )}
          <div className="mt-3 flex gap-2">
            <button type="button" className="btn-primary" disabled={busy || !path.trim()} onClick={() => void add().catch(() => {})}>
              {busy ? <Spinner size={14} /> : <Plus size={14} />} Add
            </button>
            <button type="button" className="btn-quiet" onClick={onCancel}>
              Cancel
            </button>
          </div>
        </>
      ) : (
        <>
          <input
            className="field mono mt-3 w-full"
            placeholder={"\\\\nas\\media"}
            value={path}
            onChange={(e) => setPath(e.target.value)}
            aria-label="Network share"
            autoFocus
          />
          <p className="mt-1.5 text-xs text-content-muted">
            The share's network path, not a mapped drive letter. Leave the user name empty if the share is open to everyone. The password is kept encrypted
            for this computer.
          </p>
          <ShareSignIn submitLabel="Sign in and add" disabled={!path.trim()} onCancel={onCancel} onSubmit={(user, password) => add(user, password)} />
        </>
      )}
    </div>
  );
}

/** A share's user name and password. A blank password keeps the saved one. */
function ShareSignIn({
  user: initialUser = "",
  hasPassword = false,
  submitLabel,
  disabled = false,
  onCancel,
  onSubmit,
}: {
  user?: string;
  hasPassword?: boolean;
  submitLabel: string;
  disabled?: boolean;
  onCancel: () => void;
  onSubmit: (user: string, password: string | null) => Promise<void>;
}) {
  const [user, setUser] = useState(initialUser);
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      await onSubmit(user.trim(), password || (hasPassword ? null : ""));
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form
      className="mt-3"
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <div className="grid gap-2 sm:grid-cols-2">
        <input className="field w-full" placeholder="User name (nas\you or you)" autoComplete="off" value={user} onChange={(e) => setUser(e.target.value)} aria-label="User name" />
        <input
          className="field w-full"
          type="password"
          autoComplete="new-password"
          placeholder={hasPassword ? "Saved; type to replace" : "Password"}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          aria-label="Password"
        />
      </div>
      {err && <p className="mt-2 text-xs text-critical">{err}</p>}
      <div className="mt-3 flex gap-2">
        <button type="submit" className="btn-primary" disabled={busy || disabled}>
          {busy ? <Spinner size={14} /> : <KeyRound size={14} />} {submitLabel}
        </button>
        <button type="button" className="btn-quiet" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}
