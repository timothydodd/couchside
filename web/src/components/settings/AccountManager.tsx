import { useState } from "react";
import { Check, ChevronDown, ChevronRight, CircleDot, KeyRound, Plus, Trash2, UserPlus } from "lucide-react";
import ProfileAvatar, { PROFILE_COLORS, avatarStyle } from "../ProfileAvatar";
import { MIN_PASSWORD } from "../auth/NewPassword";
import SessionList from "./SessionList";
import { ErrorNote, Segmented, Spinner } from "../ui";
import { api, useApi } from "../../lib/api";
import type { Profile, ProfileColor } from "../../lib/types";
import { authError, useAuth } from "../../stores/auth";

/**
 * Settings → Accounts (admins, with accounts on). Each profile is an account:
 * add one with a temporary password, set its role and whether it may record,
 * reset its password, disable or delete it, and sign its devices out.
 */
export default function AccountManager() {
  const { data, error, loading, reload } = useApi<Profile[]>("/api/accounts");
  const [open, setOpen] = useState<number | "new" | null>(null);
  const { passwordless, passwordlessLocked } = useAuth();
  const [toggleErr, setToggleErr] = useState<string | null>(null);

  const setPasswordless = async (enabled: boolean) => {
    setToggleErr(null);
    try {
      await api("/api/settings/passwordless", { method: "PUT", json: { enabled } });
      await useAuth.getState().load();
    } catch (e) {
      setToggleErr(authError(e));
    }
  };

  return (
    <section className="card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <div className="card-title">Accounts</div>
          <div className="text-xs text-content-muted">Everyone who can sign in. Admins can change settings and libraries.</div>
        </div>
        {open !== "new" && (
          <button className="btn-ghost !text-xs" onClick={() => setOpen("new")}>
            <UserPlus size={13} /> Add account
          </button>
        )}
      </div>
      <label className="mb-3 flex items-start gap-2 rounded-md border border-border-light p-3 text-sm text-content-secondary">
        <input
          type="checkbox"
          className="accent-brand mt-0.5"
          checked={passwordless}
          disabled={passwordlessLocked}
          onChange={(e) => void setPasswordless(e.target.checked)}
        />
        <span>
          <span className="font-medium text-content">Passwordless sign-in</span>
          <span className="block text-xs text-content-muted">
            {passwordlessLocked
              ? "Off: COUCHSIDE_AUTH=true on the server requires passwords."
              : "Anyone who can reach Couchside picks a profile to sign in. Profiles with a password still ask for it, so lock admin accounts. Turn this off before exposing Couchside to the internet."}
          </span>
        </span>
      </label>
      {toggleErr && <ErrorNote>{toggleErr}</ErrorNote>}
      {loading && !data && <Spinner />}
      {error && <ErrorNote>{error}</ErrorNote>}
      {open === "new" && (
        <AccountForm
          onDone={() => {
            setOpen(null);
            void reload();
          }}
        />
      )}
      <ul className="mt-2 divide-y divide-border-light rounded-md border border-border-light">
        {data?.map((p) => (
          <li key={p.id}>
            <button className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-muted" onClick={() => setOpen(open === p.id ? null : p.id)} aria-expanded={open === p.id}>
              {open === p.id ? <ChevronDown size={14} className="text-content-muted" /> : <ChevronRight size={14} className="text-content-muted" />}
              <ProfileAvatar profile={p} size={24} />
              <span className={`min-w-0 flex-1 truncate text-sm ${p.disabled ? "text-content-muted line-through" : "text-content"}`}>{p.name}</span>
              <AccountBadges p={p} />
            </button>
            {open === p.id && (
              <div className="border-t border-border-light bg-page/40 px-3 py-3">
                <AccountForm
                  account={p}
                  onDone={() => {
                    setOpen(null);
                    void reload();
                  }}
                  onChanged={() => void reload()}
                />
              </div>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function AccountBadges({ p }: { p: Profile }) {
  const passwordless = useAuth((s) => s.passwordless);
  return (
    <span className="flex shrink-0 items-center gap-1.5">
      {p.role === "admin" && <span className="tint-info rounded px-1.5 text-[11px] font-semibold">Admin</span>}
      {p.role !== "admin" && p.canRecord && (
        <span className="tint-muted inline-flex items-center gap-1 rounded px-1.5 text-[11px] font-semibold">
          <CircleDot size={10} /> Records
        </span>
      )}
      {!p.hasPassword && <span className={`${passwordless ? "tint-muted" : "tint-warning"} rounded px-1.5 text-[11px] font-semibold`}>No password</span>}
      {p.hasPassword && p.mustChangePassword && <span className="tint-muted rounded px-1.5 text-[11px] font-semibold">Temporary password</span>}
      {p.disabled && <span className="tint-critical rounded px-1.5 text-[11px] font-semibold">Disabled</span>}
    </span>
  );
}

/** Add an account (no account given) or edit one. */
function AccountForm({ account, onDone, onChanged }: { account?: Profile; onDone: () => void; onChanged?: () => void }) {
  const me = useAuth((s) => s.user);
  const passwordless = useAuth((s) => s.passwordless);
  const [name, setName] = useState(account?.name ?? "");
  const [color, setColor] = useState<ProfileColor>(account?.color ?? "accent");
  const [role, setRole] = useState<Profile["role"]>(account?.role ?? "user");
  const [canRecord, setCanRecord] = useState(account?.canRecord ?? false);
  const [disabled, setDisabled] = useState(account?.disabled ?? false);
  const [password, setPassword] = useState("");
  const [reset, setReset] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const self = account?.id === me?.id;

  const run = async (fn: () => Promise<unknown>, ok?: string, done = true) => {
    setBusy(true);
    setMsg(null);
    try {
      await fn();
      if (ok) setMsg({ ok: true, text: ok });
      if (done) onDone();
      else onChanged?.();
    } catch (e) {
      setMsg({ ok: false, text: authError(e) });
    } finally {
      setBusy(false);
    }
  };

  const save = () =>
    run(() =>
      account
        ? api(`/api/accounts/${account.id}`, { method: "PUT", json: { name, color, role, canRecord, disabled } })
        : api("/api/accounts", { method: "POST", json: { name, color, role, canRecord, password } }),
    );

  return (
    <form
      className={account ? "flex flex-col gap-4" : "mb-3 flex flex-col gap-4 rounded-md border border-border-light p-3"}
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      {!account && <div className="text-sm font-medium text-content">New account</div>}
      <div className="form-grid text-sm">
        <span className="text-content-muted">Name</span>
        <input className="field w-full max-w-xs" value={name} maxLength={30} autoFocus={!account} onChange={(e) => setName(e.target.value)} />
        <span className="text-content-muted">Colour</span>
        <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Colour">
          {PROFILE_COLORS.map((c) => (
            <button
              key={c.id}
              type="button"
              role="radio"
              aria-checked={color === c.id}
              aria-label={c.id}
              className={`avatar h-6 w-6 ${color === c.id ? "ring-2 ring-accent ring-offset-2 ring-offset-[var(--bg-surface)]" : ""}`}
              style={avatarStyle(c.id)}
              onClick={() => setColor(c.id)}
            >
              {color === c.id && <Check size={12} />}
            </button>
          ))}
        </div>
        <span className="text-content-muted">Role</span>
        <div>
          <Segmented<Profile["role"]>
            label="Role"
            value={role}
            onChange={setRole}
            options={[
              { id: "user", label: "User" },
              { id: "admin", label: "Admin" },
            ]}
          />
          <p className="mt-1 text-xs text-content-muted">
            {role === "admin" ? "Can change settings, libraries, files and accounts, and record." : "Can watch, and change their own preferences and password."}
          </p>
        </div>
        {role === "user" && (
          <>
            <span className="text-content-muted">Recording</span>
            <label className="flex items-center gap-2 text-content-secondary">
              <input type="checkbox" className="accent-brand" checked={canRecord} onChange={(e) => setCanRecord(e.target.checked)} />
              Can schedule recordings and series (and manage their own)
            </label>
          </>
        )}
        {account && !self && (
          <>
            <span className="text-content-muted">Status</span>
            <label className="flex items-center gap-2 text-content-secondary">
              <input type="checkbox" className="accent-brand" checked={disabled} onChange={(e) => setDisabled(e.target.checked)} />
              Disabled (can't sign in; signed out everywhere)
            </label>
          </>
        )}
        {!account && (
          <>
            <span className="text-content-muted">{passwordless ? "Password" : "Temporary password"}</span>
            <div>
              <input className="field w-full max-w-xs" type="text" autoComplete="off" value={password} onChange={(e) => setPassword(e.target.value)} />
              <p className="mt-1 text-xs text-content-muted">
                {passwordless
                  ? `Optional. Leave it empty to sign in by picking the profile; a temporary password (at least ${MIN_PASSWORD} characters) is replaced at first sign-in.`
                  : `At least ${MIN_PASSWORD} characters. They'll choose their own when they first sign in.`}
              </p>
            </div>
          </>
        )}
      </div>
      {msg && (msg.ok ? <span className="text-xs text-good">{msg.text}</span> : <ErrorNote>{msg.text}</ErrorNote>)}
      <div className="flex items-center gap-2">
        <button type="submit" className="btn-primary" disabled={busy || !name.trim() || (!account && (password !== "" || !passwordless) && password.trim().length < MIN_PASSWORD)}>
          {account ? (
            "Save"
          ) : (
            <>
              <Plus size={14} /> Add
            </>
          )}
        </button>
        <button type="button" className="btn-ghost" onClick={onDone} disabled={busy}>
          Cancel
        </button>
      </div>

      {account && (
        <>
          {!self && (
            <div className="border-t border-border-light pt-3">
              <div className="field-label">Reset password</div>
              <div className="flex flex-wrap items-center gap-2">
                <input
                  className="field w-56"
                  type="text"
                  autoComplete="off"
                  placeholder="New temporary password"
                  value={reset}
                  onChange={(e) => setReset(e.target.value)}
                />
                <button
                  type="button"
                  className="btn-ghost"
                  disabled={busy || reset.trim().length < MIN_PASSWORD}
                  onClick={() =>
                    void run(
                      () => api(`/api/accounts/${account.id}/password`, { method: "POST", json: { password: reset } }).then(() => setReset("")),
                      `${account.name} has been signed out everywhere and must choose a new password at next sign-in.`,
                      false,
                    )
                  }
                >
                  <KeyRound size={14} /> Reset
                </button>
                {passwordless && account.hasPassword && (
                  <button
                    type="button"
                    className="btn-quiet"
                    disabled={busy}
                    onClick={() =>
                      void run(
                        () => api(`/api/accounts/${account.id}/password`, { method: "POST", json: { password: "" } }),
                        `${account.name} now signs in by picking the profile.`,
                        false,
                      )
                    }
                  >
                    Remove password
                  </button>
                )}
              </div>
            </div>
          )}
          <div className="border-t border-border-light pt-3">
            <div className="field-label">Signed-in devices</div>
            <SessionList url={`/api/accounts/${account.id}/sessions`} />
          </div>
          {!self && (
            <div className="border-t border-border-light pt-3">
              <button
                type="button"
                className={confirmDelete ? "btn-danger" : "btn-quiet"}
                disabled={busy}
                onClick={() => (confirmDelete ? void run(() => api(`/api/accounts/${account.id}`, { method: "DELETE" })) : setConfirmDelete(true))}
              >
                <Trash2 size={14} /> {confirmDelete ? `Delete ${account.name} for good` : "Delete account"}
              </button>
              {confirmDelete && <p className="mt-2 text-xs text-content-muted">This removes {account.name}'s watch progress, favourite channels and settings.</p>}
            </div>
          )}
        </>
      )}
    </form>
  );
}
