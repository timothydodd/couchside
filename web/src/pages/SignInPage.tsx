import { useState } from "react";
import { ArrowLeft, Lock, LogIn } from "lucide-react";
import AuthShell from "../components/auth/AuthShell";
import ProfileAvatar from "../components/ProfileAvatar";
import { ErrorNote, WarningNote } from "../components/ui";
import type { ProfileStub } from "../lib/types";
import { authError, useAuth } from "../stores/auth";
import { useRouter } from "../stores/router";

/**
 * Who's watching. With passwordless sign-in every profile is listed and a
 * click signs in; a profile with a password asks for it. Otherwise only the
 * profiles this browser is already signed in to are listed (nobody else's
 * name is shown), plus a name-and-password form. switching = opened from the
 * sidebar.
 */
export default function SignInPage({ switching = false }: { switching?: boolean }) {
  const { passwordless, profiles, signedIn, user, insecure, login, pick, switchTo } = useAuth();
  const back = useRouter((s) => s.back);
  const tiles = passwordless ? profiles : signedIn;
  const [asking, setAsking] = useState<ProfileStub | null>(null); // the profile whose password we want
  const [form, setForm] = useState(!passwordless && signedIn.length === 0);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState<number | "form" | null>(null);
  const [err, setErr] = useState<string | null>(null);

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

  const choose = (p: ProfileStub) => {
    if (p.id === user?.id) return back("/");
    // Already signed in on this browser: switch without a password.
    if (signedIn.some((s) => s.id === p.id)) return void run(p.id, () => switchTo(p.id));
    if (passwordless && !p.hasPassword) return void run(p.id, () => pick(p.id));
    setAsking(p);
    setName(p.name);
    setPassword("");
    setErr(null);
  };

  const showForm = asking !== null || form;

  return (
    <AuthShell
      title={switching ? "Switch profile" : passwordless ? "Who's watching?" : "Sign in to Couchside"}
      subtitle={
        asking
          ? `Enter ${asking.name}'s password.`
          : passwordless
            ? "Pick your profile."
            : tiles.length
              ? "Pick a profile signed in on this browser, or sign in as someone else."
              : "Each profile keeps its own progress, favourite channels and settings."
      }
      top={
        switching && (
          <button className="btn-quiet" onClick={() => back("/")}>
            <ArrowLeft size={15} /> Back
          </button>
        )
      }
    >
      {insecure && (
        <div className="mt-6 w-full max-w-sm">
          <WarningNote>
            This connection isn't encrypted. Signing in sends your password and session across the internet in the clear. Whoever runs this
            Couchside should serve it over HTTPS.
          </WarningNote>
        </div>
      )}
      {tiles.length > 0 && !asking && (
        <div className="mt-8 flex flex-wrap justify-center gap-6">
          {tiles.map((p) => (
            <button
              key={p.id}
              className="group flex w-28 flex-col items-center gap-2 focus:outline-none"
              disabled={busy !== null}
              onClick={() => choose(p)}
              aria-label={p.hasPassword ? `${p.name} (password)` : p.name}
            >
              <span className="relative">
                <ProfileAvatar
                  profile={p}
                  size={96}
                  className={`transition-transform group-hover:scale-105 group-focus-visible:scale-105 group-focus-visible:ring-2 group-focus-visible:ring-accent ${p.id === user?.id ? "ring-2 ring-accent ring-offset-2 ring-offset-[var(--bg-page)]" : ""} ${busy === p.id ? "animate-pulse" : ""}`}
                />
                {p.hasPassword && !signedIn.some((s) => s.id === p.id) && (
                  <span className="absolute -bottom-1 -right-1 flex h-6 w-6 items-center justify-center rounded-full border border-border-light bg-surface text-content-secondary">
                    <Lock size={12} />
                  </span>
                )}
              </span>
              <span className="max-w-full truncate text-sm text-content-secondary group-hover:text-content">{p.name}</span>
            </button>
          ))}
        </div>
      )}
      {showForm ? (
        <form
          className="card mt-8 flex w-full max-w-sm flex-col gap-4 p-6"
          onSubmit={(e) => {
            e.preventDefault();
            void run("form", () => login(name, password));
          }}
        >
          {asking ? (
            <div className="flex items-center gap-3">
              <ProfileAvatar profile={asking} size={40} />
              <span className="font-medium text-content">{asking.name}</span>
            </div>
          ) : (
            <label className="block">
              <span className="field-label">Name</span>
              <input className="field w-full" autoComplete="username" autoFocus value={name} onChange={(e) => setName(e.target.value)} />
            </label>
          )}
          <label className="block">
            <span className="field-label">Password</span>
            <input
              className="field w-full"
              type="password"
              autoComplete="current-password"
              autoFocus={!!asking}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          {err && <ErrorNote>{err}</ErrorNote>}
          <button type="submit" className="btn-primary justify-center" disabled={busy !== null || !name.trim() || !password}>
            <LogIn size={15} /> Sign in
          </button>
          {asking ? (
            <button type="button" className="btn-quiet justify-center" onClick={() => setAsking(null)}>
              <ArrowLeft size={14} /> Pick another profile
            </button>
          ) : (
            <p className="text-xs text-content-muted">Forgotten your password? Ask whoever runs this Couchside to reset it.</p>
          )}
        </form>
      ) : (
        <>
          {err && (
            <div className="mt-6 w-full max-w-sm">
              <ErrorNote>{err}</ErrorNote>
            </div>
          )}
          {!passwordless && (
            <button className="btn-ghost mt-10" onClick={() => setForm(true)}>
              <LogIn size={14} /> Sign in as someone else
            </button>
          )}
        </>
      )}
    </AuthShell>
  );
}
