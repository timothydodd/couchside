import { useState } from "react";
import { ShieldCheck } from "lucide-react";
import AuthShell from "../components/auth/AuthShell";
import NewPassword from "../components/auth/NewPassword";
import { ErrorNote } from "../components/ui";
import { authError, useAuth } from "../stores/auth";

/** First run with accounts on: the setup code from the server log creates the first admin. */
export default function SetupPage() {
  const setup = useAuth((s) => s.setup);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (!password) return;
    setBusy(true);
    setErr(null);
    try {
      await setup(code, name, password);
    } catch (e) {
      setErr(authError(e));
      setBusy(false);
    }
  };

  return (
    <AuthShell title="Set up accounts" subtitle="Accounts are on, and this server has no admin yet. Create yours to finish.">
      <form
        className="card mt-8 flex w-full max-w-sm flex-col gap-4 p-6"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label className="block">
          <span className="field-label">Setup code</span>
          <input className="field mono w-full uppercase" autoFocus autoComplete="off" value={code} onChange={(e) => setCode(e.target.value)} placeholder="XXXX-XXXX-XXXX" />
          <span className="mt-1 block text-xs text-content-muted">
            Printed in the server log when it started (<span className="mono">kubectl logs deploy/couchside</span> or{" "}
            <span className="mono">docker compose logs</span>).
          </span>
        </label>
        <label className="block">
          <span className="field-label">Your name</span>
          <input className="field w-full" autoComplete="username" maxLength={30} value={name} onChange={(e) => setName(e.target.value)} />
          <span className="mt-1 block text-xs text-content-muted">Use an existing profile's name to keep its watch history.</span>
        </label>
        <NewPassword label="Password" onChange={setPassword} />
        {err && <ErrorNote>{err}</ErrorNote>}
        <button type="submit" className="btn-primary justify-center" disabled={busy || !code.trim() || !name.trim() || !password}>
          <ShieldCheck size={15} /> Create admin and sign in
        </button>
      </form>
    </AuthShell>
  );
}
