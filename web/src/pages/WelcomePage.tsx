import { useState } from "react";
import { ArrowRight } from "lucide-react";
import AuthShell from "../components/auth/AuthShell";
import NewPassword from "../components/auth/NewPassword";
import { ErrorNote, Spinner } from "../components/ui";
import { authError, useAuth } from "../stores/auth";
import SignInPage from "./SignInPage";

/**
 * First run, step 1 of 2: who you are. Names the server's admin profile and
 * signs you in, with a password if you want one (worth it when other people
 * or the internet can reach the server). Step 2 is FirstRunMediaPage.
 */
export default function WelcomePage() {
  const welcome = useAuth((s) => s.welcome);
  const profiles = useAuth((s) => s.profiles);
  const [name, setName] = useState(profiles.length === 1 && profiles[0].name !== "Me" ? profiles[0].name : "");
  const [protect, setProtect] = useState(false);
  const [password, setPassword] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [signIn, setSignIn] = useState(false);
  if (signIn) return <SignInPage />;

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      await welcome(name.trim(), protect ? (password ?? "") : "");
    } catch (e) {
      setErr(authError(e));
      setBusy(false);
    }
  };

  return (
    <AuthShell title="Welcome to Couchside" subtitle="Two quick steps: who you are, then where your movies and TV shows are.">
      <form
        className="card mt-8 flex w-full max-w-sm flex-col gap-4 p-6"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <div className="text-xs font-semibold uppercase tracking-wide text-content-muted">Step 1 of 2</div>
        <label className="block">
          <span className="field-label">Your name</span>
          <input className="field w-full" autoFocus autoComplete="username" maxLength={30} value={name} onChange={(e) => setName(e.target.value)} />
          <span className="mt-1 block text-xs text-content-muted">You're the admin: you'll set up libraries and add the rest of the household later.</span>
        </label>
        <label className="flex items-start gap-2 text-sm">
          <input type="checkbox" className="mt-1" checked={protect} onChange={(e) => setProtect(e.target.checked)} />
          <span>
            Protect it with a password
            <span className="block text-xs text-content-muted">Recommended if anyone outside your home can reach this server. Without one, you pick your name to sign in.</span>
          </span>
        </label>
        {protect && <NewPassword label="Password" onChange={setPassword} />}
        {err && <ErrorNote>{err}</ErrorNote>}
        <button type="submit" className="btn-primary justify-center" disabled={busy || !name.trim() || (protect && !password)}>
          {busy ? <Spinner size={14} /> : <ArrowRight size={15} />} Next
        </button>
        {err && (
          <button type="button" className="btn-quiet justify-center !text-xs" onClick={() => setSignIn(true)}>
            Already set up? Sign in instead
          </button>
        )}
      </form>
    </AuthShell>
  );
}
