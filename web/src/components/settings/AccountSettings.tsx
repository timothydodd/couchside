import { useState } from "react";
import { KeyRound, LogOut } from "lucide-react";
import NewPassword from "../auth/NewPassword";
import SessionList from "./SessionList";
import { ErrorNote } from "../ui";
import { api } from "../../lib/api";
import { authError, useAuth } from "../../stores/auth";

/** Your own account: password, signed-in devices, sign out. Only with accounts on. */
export default function AccountSettings() {
  const { user, changePassword, logout } = useAuth();
  const hasPassword = !!user?.hasPassword;
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState<string | null>(null);
  const [formKey, setFormKey] = useState(0);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [devicesKey, setDevicesKey] = useState(0);

  const save = async () => {
    if (!password) return;
    setBusy(true);
    setMsg(null);
    try {
      await changePassword(current, password);
      setCurrent("");
      setPassword(null);
      setFormKey((k) => k + 1);
      setDevicesKey((k) => k + 1);
      setMsg({
        ok: true,
        text: hasPassword ? "Password changed. Your other devices have been signed out." : "Password set. Picking your profile now asks for it.",
      });
      void useAuth.getState().load();
    } catch (e) {
      setMsg({ ok: false, text: authError(e) });
    } finally {
      setBusy(false);
    }
  };

  const signOutOthers = async () => {
    await api("/api/auth/sessions/others/end", { method: "POST" });
    setDevicesKey((k) => k + 1);
  };

  return (
    <section className="card p-4">
      <div className="mb-4 flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <div className="card-title">Your account</div>
          <div className="text-xs text-content-muted">
            Signed in as {user?.name}
            {user?.role === "admin" ? " (admin)" : ""}.
          </div>
        </div>
        <button className="btn-ghost !text-xs" onClick={() => void logout()}>
          <LogOut size={13} /> Sign out
        </button>
      </div>

      <form
        key={formKey}
        className="grid gap-3 sm:grid-cols-3"
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        {hasPassword ? (
          <label className="block">
            <span className="field-label">Current password</span>
            <input className="field w-full" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
          </label>
        ) : (
          <p className="text-xs text-content-muted sm:self-center">
            You sign in by picking your profile. Set a password to lock it; others will need it to use your profile.
          </p>
        )}
        <NewPassword onChange={setPassword} />
        <div className="flex items-center gap-3 sm:col-span-3">
          <button type="submit" className="btn-primary" disabled={busy || (hasPassword && !current) || !password}>
            <KeyRound size={14} /> {hasPassword ? "Change password" : "Set a password"}
          </button>
          {msg && (msg.ok ? <span className="text-xs text-good">{msg.text}</span> : <ErrorNote>{msg.text}</ErrorNote>)}
        </div>
      </form>

      <div className="mb-2 mt-6 flex items-center justify-between">
        <div className="field-label !mb-0">Signed-in devices</div>
        <button className="btn-quiet !text-xs" onClick={() => void signOutOthers()}>
          Sign out everywhere else
        </button>
      </div>
      <SessionList key={devicesKey} url="/api/auth/sessions" />
    </section>
  );
}
