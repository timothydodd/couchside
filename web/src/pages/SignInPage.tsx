import { useState } from "react";
import { ArrowLeft, LogIn } from "lucide-react";
import AuthShell from "../components/auth/AuthShell";
import ProfileAvatar from "../components/ProfileAvatar";
import { ErrorNote } from "../components/ui";
import { authError, useAuth } from "../stores/auth";
import { useRouter } from "../stores/router";

/**
 * Sign in with a name and password. Profiles this browser is already signed in
 * to are shown so a shared computer can switch without retyping passwords;
 * nobody else's name is ever listed. switching = opened from the sidebar.
 */
export default function SignInPage({ switching = false }: { switching?: boolean }) {
  const { signedIn, user, login, switchTo } = useAuth();
  const back = useRouter((s) => s.back);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState<number | "form" | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [form, setForm] = useState(signedIn.length === 0);

  const run = async (which: number | "form", fn: () => Promise<void>) => {
    setBusy(which);
    setErr(null);
    try {
      await fn();
    } catch (e) {
      setErr(authError(e));
      setBusy(null);
    }
  };

  return (
    <AuthShell
      title={switching ? "Switch profile" : "Sign in to Couchside"}
      subtitle={signedIn.length ? "Pick a profile signed in on this browser, or sign in as someone else." : "Each profile keeps its own progress, favourite channels and settings."}
      top={
        switching && (
          <button className="btn-quiet" onClick={() => back("/")}>
            <ArrowLeft size={15} /> Back
          </button>
        )
      }
    >
      {signedIn.length > 0 && (
        <div className="mt-8 flex flex-wrap justify-center gap-6">
          {signedIn.map((p) => (
            <button
              key={p.id}
              className="group flex w-28 flex-col items-center gap-2 focus:outline-none"
              disabled={busy !== null}
              onClick={() => (p.id === user?.id ? back("/") : void run(p.id, () => switchTo(p.id)))}
            >
              <ProfileAvatar
                profile={p}
                size={96}
                className={`transition-transform group-hover:scale-105 group-focus-visible:scale-105 ${p.id === user?.id ? "ring-2 ring-accent ring-offset-2 ring-offset-[var(--bg-page)]" : ""} ${busy === p.id ? "animate-pulse" : ""}`}
              />
              <span className="max-w-full truncate text-sm text-content-secondary group-hover:text-content">{p.name}</span>
            </button>
          ))}
        </div>
      )}
      {form ? (
        <form
          className="card mt-8 flex w-full max-w-sm flex-col gap-4 p-6"
          onSubmit={(e) => {
            e.preventDefault();
            void run("form", () => login(name, password));
          }}
        >
          <label className="block">
            <span className="field-label">Name</span>
            <input className="field w-full" autoComplete="username" autoFocus value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="block">
            <span className="field-label">Password</span>
            <input className="field w-full" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          {err && <ErrorNote>{err}</ErrorNote>}
          <button type="submit" className="btn-primary justify-center" disabled={busy !== null || !name.trim() || !password}>
            <LogIn size={15} /> Sign in
          </button>
          <p className="text-xs text-content-muted">Forgotten your password? Ask whoever runs this Couchside to reset it.</p>
        </form>
      ) : (
        <>
          {err && (
            <div className="mt-6 w-full max-w-sm">
              <ErrorNote>{err}</ErrorNote>
            </div>
          )}
          <button className="btn-ghost mt-10" onClick={() => setForm(true)}>
            <LogIn size={14} /> Sign in as someone else
          </button>
        </>
      )}
    </AuthShell>
  );
}
