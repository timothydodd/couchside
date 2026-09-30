import { useState } from "react";
import { ArrowLeft, Check, Pencil, Plus, Trash2 } from "lucide-react";
import ProfileAvatar, { PROFILE_COLORS, avatarStyle } from "../components/ProfileAvatar";
import { ErrorNote } from "../components/ui";
import type { Profile, ProfileColor } from "../lib/types";
import { useProfile } from "../stores/profile";
import { useRouter } from "../stores/router";

type Editing = { kind: "new" } | { kind: "edit"; profile: Profile } | null;

/**
 * "Who's watching?": pick a profile, or add, rename, recolour and delete them.
 * Shown full screen at start-up when several profiles exist and this browser
 * hasn't picked one, and from the sidebar to switch.
 */
export default function ProfilesPage() {
  const { profiles, current, chosen, select } = useProfile();
  const { back } = useRouter();
  const [manage, setManage] = useState(false);
  const [editing, setEditing] = useState<Editing>(null);
  const [busy, setBusy] = useState<number | null>(null);
  // Someone who already has a profile in this browser came here to switch.
  const canLeave = chosen || profiles.length <= 1;

  const pick = async (p: Profile) => {
    if (manage) return setEditing({ kind: "edit", profile: p });
    setBusy(p.id);
    await select(p.id).catch(() => setBusy(null));
  };

  return (
    <div className="flex h-full flex-col items-center overflow-auto bg-page px-6 py-10">
      {canLeave && !editing && (
        <button className="btn-quiet self-start" onClick={() => back("/")}>
          <ArrowLeft size={15} /> Back
        </button>
      )}
      <div className="my-auto flex w-full max-w-3xl flex-col items-center py-8">
        <img src="/icons/logo-64.png" alt="" className="mb-6 h-10 w-10" />
        {editing ? (
          <ProfileEditor editing={editing} onDone={() => setEditing(null)} />
        ) : (
          <>
            <h1 className="text-2xl font-semibold text-content">{manage ? "Manage profiles" : "Who's watching?"}</h1>
            <p className="mt-1 text-sm text-content-muted">
              {manage ? "Pick a profile to rename, recolour or delete it." : "Each profile keeps its own progress, favourite channels and settings."}
            </p>
            <div className="mt-8 flex flex-wrap justify-center gap-6">
              {profiles.map((p) => (
                <button key={p.id} className="group flex w-28 flex-col items-center gap-2 focus:outline-none" onClick={() => void pick(p)} disabled={busy !== null}>
                  <span className="relative">
                    <ProfileAvatar
                      profile={p}
                      size={96}
                      className={`transition-transform group-hover:scale-105 group-focus-visible:scale-105 ${chosen && p.id === current?.id ? "ring-2 ring-accent ring-offset-2 ring-offset-[var(--bg-page)]" : ""} ${busy === p.id ? "animate-pulse" : ""}`}
                    />
                    {manage && (
                      <span className="absolute inset-0 flex items-center justify-center rounded-md bg-black/45 text-white">
                        <Pencil size={22} />
                      </span>
                    )}
                  </span>
                  <span className="max-w-full truncate text-sm text-content-secondary group-hover:text-content">{p.name}</span>
                </button>
              ))}
              {!manage && (
                <button className="group flex w-28 flex-col items-center gap-2 focus:outline-none" onClick={() => setEditing({ kind: "new" })}>
                  <span className="flex h-24 w-24 items-center justify-center rounded-md border border-dashed border-border text-content-muted transition-colors group-hover:border-accent group-hover:text-content group-focus-visible:border-accent">
                    <Plus size={30} />
                  </span>
                  <span className="text-sm text-content-muted group-hover:text-content">Add profile</span>
                </button>
              )}
            </div>
            <button className="btn-ghost mt-10" onClick={() => setManage((m) => !m)}>
              {manage ? (
                <>
                  <Check size={14} /> Done
                </>
              ) : (
                <>
                  <Pencil size={14} /> Manage profiles
                </>
              )}
            </button>
          </>
        )}
      </div>
    </div>
  );
}

function ProfileEditor({ editing, onDone }: { editing: NonNullable<Editing>; onDone: () => void }) {
  const { profiles, create, update, remove } = useProfile();
  const existing = editing.kind === "edit" ? editing.profile : null;
  const [name, setName] = useState(existing?.name ?? "");
  // A new profile starts on the first colour nobody uses yet.
  const unused = PROFILE_COLORS.find((c) => !profiles.some((p) => p.color === c.id));
  const [color, setColor] = useState<ProfileColor>(existing?.color ?? unused?.id ?? PROFILE_COLORS[profiles.length % PROFILE_COLORS.length].id);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  };
  const save = () => run(() => (existing ? update(existing.id, name, color) : create(name, color)));

  return (
    <form
      className="card flex w-full max-w-sm flex-col items-center p-6"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <div className="card-title self-start">{existing ? "Edit profile" : "Add a profile"}</div>
      <ProfileAvatar profile={{ name: name || "?", color }} size={80} className="mt-4" />
      <label className="mt-5 w-full">
        <span className="field-label">Name</span>
        <input className="field w-full" value={name} maxLength={30} autoFocus onChange={(e) => setName(e.target.value)} placeholder="e.g. Sam" />
      </label>
      <div className="mt-4 w-full">
        <span className="field-label">Colour</span>
        <div className="flex flex-wrap gap-2" role="radiogroup" aria-label="Colour">
          {PROFILE_COLORS.map((c) => (
            <button
              key={c.id}
              type="button"
              role="radio"
              aria-checked={color === c.id}
              aria-label={c.id}
              className={`avatar h-8 w-8 ${color === c.id ? "ring-2 ring-accent ring-offset-2 ring-offset-[var(--bg-surface)]" : ""}`}
              style={avatarStyle(c.id)}
              onClick={() => setColor(c.id)}
            >
              {color === c.id && <Check size={14} />}
            </button>
          ))}
        </div>
      </div>
      {err && (
        <div className="mt-4 w-full">
          <ErrorNote>{err}</ErrorNote>
        </div>
      )}
      <div className="mt-6 flex w-full items-center gap-2">
        {existing && profiles.length > 1 && (
          <button
            type="button"
            className={confirmDelete ? "btn-danger" : "btn-quiet"}
            disabled={busy}
            onClick={() => (confirmDelete ? void run(() => remove(existing.id)) : setConfirmDelete(true))}
            title="Deletes this profile's progress, favourites and settings"
          >
            <Trash2 size={14} /> {confirmDelete ? "Delete for good" : "Delete"}
          </button>
        )}
        <div className="flex-1" />
        <button type="button" className="btn-ghost" onClick={onDone} disabled={busy}>
          Cancel
        </button>
        <button type="submit" className="btn-primary" disabled={busy || !name.trim()}>
          {existing ? "Save" : "Add"}
        </button>
      </div>
      {confirmDelete && <p className="mt-3 text-xs text-content-muted">This removes {existing?.name}'s watch progress, favourite channels and settings.</p>}
    </form>
  );
}
