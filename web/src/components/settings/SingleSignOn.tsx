import { useEffect, useState } from "react";
import { ErrorNote } from "../ui";
import { api, useApi } from "../../lib/api";
import { errText } from "../../lib/errors";

interface Config {
  issuer: string;
  clientId: string;
  clientSecret?: string;
  hasSecret: boolean;
  claim: string;
  create: boolean;
  adminGroup: string;
  label: string;
}

/**
 * Settings → Accounts: sign in through an OpenID Connect provider (Authelia,
 * Authentik, Keycloak, Google…). Whoever the provider says someone is gets
 * matched to a profile by name.
 */
export default function SingleSignOn() {
  const { data, error, reload } = useApi<{ config: Config; redirectUri: string }>("/api/settings/oidc");
  const [draft, setDraft] = useState<Config | null>(null);
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  useEffect(() => setDraft(data?.config ?? null), [data]);

  const save = async (c: Config) => {
    setBusy(true);
    setMsg(null);
    try {
      await api("/api/settings/oidc", { method: "PUT", json: { ...c, clientSecret: secret } });
      setSecret("");
      setMsg({ ok: true, text: c.issuer ? "Saved. The sign-in page now has the button." : "Single sign-on is off." });
      await reload();
    } catch (e) {
      setMsg({ ok: false, text: errText(e) });
    } finally {
      setBusy(false);
    }
  };

  const set = (patch: Partial<Config>) => draft && setDraft({ ...draft, ...patch });
  const on = !!data?.config.issuer;

  return (
    <section className="card p-4">
      <div className="card-title">Single sign-on</div>
      <p className="mb-3 mt-1 text-xs text-content-muted">
        Let people sign in through an OpenID Connect provider you already run. Whoever it says they are is matched to a profile with the same name. Passwords
        here keep working.
      </p>
      {error && !data && <ErrorNote>Couldn't load the settings: {error}</ErrorNote>}
      {draft && data && (
        <form
          className="grid gap-x-4 gap-y-3 text-sm sm:grid-cols-[140px_1fr] sm:items-center"
          onSubmit={(e) => {
            e.preventDefault();
            void save(draft);
          }}
        >
          <label className="text-content-muted" htmlFor="oidc-issuer">
            Provider address
          </label>
          <input id="oidc-issuer" className="field mono" placeholder="https://auth.example.com" value={draft.issuer} onChange={(e) => set({ issuer: e.target.value })} />
          <label className="text-content-muted" htmlFor="oidc-client">
            Client id
          </label>
          <input id="oidc-client" className="field mono" value={draft.clientId} onChange={(e) => set({ clientId: e.target.value })} />
          <label className="text-content-muted" htmlFor="oidc-secret">
            Client secret
          </label>
          <input
            id="oidc-secret"
            className="field mono"
            type="password"
            autoComplete="off"
            placeholder={draft.hasSecret ? "Saved; type to replace it" : ""}
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
          <span className="text-content-muted">Redirect address</span>
          <div>
            <span className="mono break-all text-content-secondary">{data.redirectUri}</span>
            <p className="text-xs text-content-muted">Give this to the provider as the allowed redirect (callback) address.</p>
          </div>
          <label className="text-content-muted" htmlFor="oidc-claim">
            Name comes from
          </label>
          <div>
            <input id="oidc-claim" className="field mono" value={draft.claim} onChange={(e) => set({ claim: e.target.value })} />
            <p className="text-xs text-content-muted">The claim holding the profile name: preferred_username, email or name.</p>
          </div>
          <span className="text-content-muted">New people</span>
          <label className="flex items-center gap-2 text-content-secondary">
            <input type="checkbox" className="accent-brand" checked={draft.create} onChange={(e) => set({ create: e.target.checked })} />
            Make a profile for someone the provider signs in who doesn't have one
          </label>
          <label className="text-content-muted" htmlFor="oidc-admins">
            Admin group
          </label>
          <div>
            <input id="oidc-admins" className="field mono" value={draft.adminGroup} onChange={(e) => set({ adminGroup: e.target.value })} />
            <p className="text-xs text-content-muted">Optional. New profiles for people in this group (the "groups" claim) are admins.</p>
          </div>
          <label className="text-content-muted" htmlFor="oidc-label">
            Button text
          </label>
          <input id="oidc-label" className="field" value={draft.label} onChange={(e) => set({ label: e.target.value })} />
          <span />
          <div className="flex flex-wrap items-center gap-2">
            <button type="submit" className="btn-primary" disabled={busy}>
              Save
            </button>
            {on && (
              <button type="button" className="btn-ghost" disabled={busy} onClick={() => void save({ ...draft, issuer: "" })}>
                Turn off
              </button>
            )}
            {msg && <span className={`text-xs ${msg.ok ? "text-good" : "text-critical"}`}>{msg.text}</span>}
          </div>
        </form>
      )}
    </section>
  );
}
