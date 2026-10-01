import { useState } from "react";

export const MIN_PASSWORD = 8;

/** New password + confirmation. Reports the password once both agree and it's long enough, else null. */
export default function NewPassword({ label = "New password", onChange }: { label?: string; onChange: (pw: string | null) => void }) {
  const [pw, setPw] = useState("");
  const [again, setAgain] = useState("");
  const short = pw.trim().length > 0 && pw.trim().length < MIN_PASSWORD;
  const mismatch = again.length > 0 && again !== pw;
  const update = (a: string, b: string) => onChange(a.trim().length >= MIN_PASSWORD && a === b ? a : null);
  return (
    <>
      <label className="block">
        <span className="field-label">{label}</span>
        <input
          className="field w-full"
          type="password"
          autoComplete="new-password"
          value={pw}
          onChange={(e) => {
            setPw(e.target.value);
            update(e.target.value, again);
          }}
        />
        <span className={`mt-1 block text-xs ${short ? "text-warning" : "text-content-muted"}`}>At least {MIN_PASSWORD} characters.</span>
      </label>
      <label className="block">
        <span className="field-label">Type it again</span>
        <input
          className="field w-full"
          type="password"
          autoComplete="new-password"
          value={again}
          onChange={(e) => {
            setAgain(e.target.value);
            update(pw, e.target.value);
          }}
        />
        {mismatch && <span className="mt-1 block text-xs text-warning">The passwords don't match.</span>}
      </label>
    </>
  );
}
