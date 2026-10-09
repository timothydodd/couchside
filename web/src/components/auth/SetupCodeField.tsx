/** The one-time setup code the server prints to its log while it isn't set up. */
export default function SetupCodeField({ value, onChange, autoFocus }: { value: string; onChange: (code: string) => void; autoFocus?: boolean }) {
  return (
    <label className="block">
      <span className="field-label">Setup code</span>
      <input className="field mono w-full uppercase" autoFocus={autoFocus} autoComplete="off" value={value} onChange={(e) => onChange(e.target.value)} placeholder="XXXX-XXXX-XXXX" />
      <span className="mt-1 block text-xs text-content-muted">
        Printed in the server log when it started (<span className="mono">kubectl logs deploy/couchside</span>,{" "}
        <span className="mono">docker compose logs</span>, or <span className="mono">data\logs\couchside.log</span> on Windows).
      </span>
    </label>
  );
}
