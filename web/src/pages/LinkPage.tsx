import { useState } from "react";
import { Tv } from "lucide-react";
import { ErrorNote, PageHeader } from "../components/ui";
import { api } from "../lib/api";
import { errText } from "../lib/errors";
import { useProfile } from "../stores/profile";

/**
 * /link: sign a TV in from here. The TV app shows a code; entering it signs
 * the TV in as the profile using this browser.
 */
export default function LinkPage() {
  const profile = useProfile((s) => s.current);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const r = await api<{ device: string }>("/api/auth/device/approve", { method: "POST", json: { userCode: code } });
      setDone(r.device || "Your TV");
      setCode("");
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <PageHeader title="Sign in a TV" subtitle="Enter the code the TV app is showing." />
      <div className="gutter max-w-md py-5">
        <form
          className="card flex flex-col gap-4 p-5"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <p className="text-sm text-content-secondary">
            The TV will be signed in as <span className="font-medium text-content">{profile?.name}</span>. Switch profile first to sign it in as someone else.
          </p>
          <label className="block">
            <span className="field-label">Code</span>
            <input
              className="field mono w-full text-center !text-lg uppercase tracking-widest"
              autoFocus
              autoComplete="off"
              autoCapitalize="characters"
              placeholder="ABCD-EFGH"
              maxLength={11}
              value={code}
              onChange={(e) => {
                setCode(e.target.value);
                setDone(null);
              }}
            />
          </label>
          {err && <ErrorNote>{err}</ErrorNote>}
          {done && (
            <div className="tint-good rounded-md px-3 py-2 text-sm">
              <Tv size={15} className="mr-1.5 inline" />
              {done} is signed in. It may take a few seconds to catch up.
            </div>
          )}
          <button type="submit" className="btn-primary justify-center" disabled={busy || code.replace(/[^a-z0-9]/gi, "").length !== 8}>
            Sign in the TV
          </button>
        </form>
      </div>
    </div>
  );
}
