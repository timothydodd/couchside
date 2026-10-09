import { useState } from "react";
import { KeyRound, LogOut } from "lucide-react";
import AuthShell from "../components/auth/AuthShell";
import NewPassword from "../components/auth/NewPassword";
import { ErrorNote } from "../components/ui";
import { authError, useAuth } from "../stores/auth";
import { attempt } from "../lib/notices";
import { useTitle } from "../lib/title";
import { takeReturnTo } from "../lib/returnTo";

/** After an admin sets a temporary password, the user picks their own before anything else. */
export default function ChangePasswordPage() {
  useTitle("Change password");
  const { user, changePassword, logout } = useAuth();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (!password) return;
    setBusy(true);
    setErr(null);
    try {
      await changePassword(current, password);
      location.assign(takeReturnTo());
    } catch (e) {
      setErr(authError(e));
      setBusy(false);
    }
  };

  return (
    <AuthShell title={`Choose your password, ${user?.name ?? ""}`} subtitle="You signed in with a temporary password. Pick one only you know.">
      <form
        className="card mt-8 flex w-full max-w-sm flex-col gap-4 p-6"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label className="block">
          <span className="field-label">Temporary password</span>
          <input className="field w-full" type="password" autoComplete="current-password" autoFocus value={current} onChange={(e) => setCurrent(e.target.value)} />
        </label>
        <NewPassword onChange={setPassword} />
        {err && <ErrorNote>{err}</ErrorNote>}
        <button type="submit" className="btn-primary justify-center" disabled={busy || !current || !password}>
          <KeyRound size={15} /> Save password
        </button>
        <button type="button" className="btn-quiet justify-center" onClick={attempt("Couldn't sign out", () => logout())}>
          <LogOut size={14} /> Sign out
        </button>
      </form>
    </AuthShell>
  );
}
