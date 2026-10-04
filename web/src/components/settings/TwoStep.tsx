import { useMemo, useState } from "react";
import qrcode from "qrcode-generator";
import { ShieldCheck } from "lucide-react";
import { ErrorNote } from "../ui";
import { api, useApi } from "../../lib/api";
import { errText } from "../../lib/errors";

interface Status {
  enabled: boolean;
  recoveryCodesLeft: number;
}

/**
 * Your account → two-step sign-in: after the password, a code from an
 * authenticator app. Turning it on shows a QR code for the app, checks a
 * code from it, then shows recovery codes once.
 */
export default function TwoStep({ hasPassword }: { hasPassword: boolean }) {
  const { data, reload } = useApi<Status>("/api/auth/totp", { fresh: true });
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null);
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState<string[] | null>(null);
  const [turningOff, setTurningOff] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };
  const start = () => run(async () => setSetup(await api<{ secret: string; uri: string }>("/api/auth/totp/setup", { method: "POST" })));
  const enable = () =>
    run(async () => {
      const r = await api<{ recoveryCodes: string[] }>("/api/auth/totp/enable", { method: "POST", json: { code } });
      setRecovery(r.recoveryCodes);
      setSetup(null);
      setCode("");
      await reload();
    });
  const disable = () =>
    run(async () => {
      await api("/api/auth/totp/disable", { method: "POST", json: { code } });
      setTurningOff(false);
      setRecovery(null);
      setCode("");
      await reload();
    });

  // The QR code as an SVG path of its dark squares.
  const qr = useMemo(() => {
    if (!setup) return null;
    const q = qrcode(0, "M");
    q.addData(setup.uri);
    q.make();
    const n = q.getModuleCount();
    let d = "";
    for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) if (q.isDark(y, x)) d += `M${x} ${y}h1v1h-1z`;
    return { n, d };
  }, [setup]);

  if (!data) return null;
  return (
    <div className="mt-6">
      <div className="mb-2 flex items-center justify-between gap-3">
        <div>
          <div className="text-sm font-medium text-content">Two-step sign-in</div>
          <div className="text-xs text-content-muted">
            {data.enabled
              ? `On. Signing in with your password also asks for a code from your authenticator app. ${data.recoveryCodesLeft} recovery code${data.recoveryCodesLeft === 1 ? "" : "s"} left.`
              : "After your password, a code from an authenticator app on your phone."}
          </div>
        </div>
        {data.enabled
          ? !turningOff && (
              <button className="btn-ghost !text-xs" onClick={() => setTurningOff(true)}>
                Turn off
              </button>
            )
          : !setup && (
              <button className="btn-ghost !text-xs" disabled={busy || !hasPassword} title={hasPassword ? undefined : "Set a password first"} onClick={() => void start()}>
                <ShieldCheck size={13} /> Turn on
              </button>
            )}
      </div>

      {setup && qr && (
        <form
          className="flex flex-wrap items-start gap-4 rounded-md border border-border-light p-3"
          onSubmit={(e) => {
            e.preventDefault();
            void enable();
          }}
        >
          {/* Dark on light whatever the theme: phone cameras need the contrast. */}
          <svg viewBox={`-2 -2 ${qr.n + 4} ${qr.n + 4}`} className="h-40 w-40 shrink-0 rounded" role="img" aria-label="QR code for your authenticator app">
            <rect x={-2} y={-2} width={qr.n + 4} height={qr.n + 4} fill="#fff" />
            <path d={qr.d} fill="#000" />
          </svg>
          <div className="min-w-0 flex-1 text-sm">
            <p className="text-content-secondary">Scan this with an authenticator app (or enter the key by hand), then type the code it shows.</p>
            <p className="mono mt-2 break-all text-xs text-content-muted">{setup.secret}</p>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <input
                className="field mono w-32 text-center"
                inputMode="numeric"
                autoComplete="one-time-code"
                placeholder="123456"
                maxLength={7}
                value={code}
                onChange={(e) => setCode(e.target.value)}
                aria-label="Code from the app"
              />
              <button type="submit" className="btn-primary" disabled={busy || code.replace(/\s/g, "").length !== 6}>
                Turn on
              </button>
              <button
                type="button"
                className="btn-quiet !text-xs"
                onClick={() => {
                  setSetup(null);
                  setCode("");
                  void api("/api/auth/totp/disable", { method: "POST", json: { code: "" } }).catch(() => {});
                }}
              >
                Cancel
              </button>
            </div>
          </div>
        </form>
      )}

      {recovery && (
        <div className="tint-warning rounded-md px-3 py-3 text-sm">
          <div className="font-medium">Save these recovery codes now. They aren't shown again.</div>
          <p className="mt-1 text-xs">Each works once in place of a code from the app, if you lose your phone.</p>
          <div className="mono mt-2 grid grid-cols-2 gap-x-6 gap-y-1 sm:grid-cols-4">
            {recovery.map((c) => (
              <span key={c}>{c}</span>
            ))}
          </div>
          <button className="btn-quiet mt-2 !text-xs" onClick={() => setRecovery(null)}>
            I've saved them
          </button>
        </div>
      )}

      {turningOff && (
        <form
          className="flex flex-wrap items-center gap-2 rounded-md border border-border-light p-3 text-sm"
          onSubmit={(e) => {
            e.preventDefault();
            void disable();
          }}
        >
          <span className="text-content-secondary">A code from the app, or a recovery code:</span>
          <input className="field mono w-36 text-center" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} aria-label="Code" autoFocus />
          <button type="submit" className="btn-danger" disabled={busy || !code.trim()}>
            Turn off
          </button>
          <button type="button" className="btn-quiet !text-xs" onClick={() => setTurningOff(false)}>
            Cancel
          </button>
        </form>
      )}
      {err && (
        <div className="mt-2">
          <ErrorNote>{err}</ErrorNote>
        </div>
      )}
    </div>
  );
}
