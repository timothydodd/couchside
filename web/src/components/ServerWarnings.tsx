import { TriangleAlert } from "lucide-react";
import Link from "./Link";
import { useIsAdmin } from "../stores/auth";
import { useStatus } from "../stores/status";

/**
 * Admin-only warnings that would otherwise sit in the log: a network share
 * the server couldn't sign in to at start-up leaves every file on it
 * unplayable while the library looks fine, so say so on every page until
 * it's fixed in Settings → Server.
 */
export default function ServerWarnings() {
  const admin = useIsAdmin();
  const shares = useStatus((s) => s.status?.shareErrors);
  if (!admin || !shares?.length) return null;
  return (
    <div className="gutter shrink-0 pt-3">
      <div className="tint-warning flex items-start gap-2 rounded-md px-3 py-2 text-xs" role="alert">
        <TriangleAlert size={14} className="mt-0.5 shrink-0" />
        <div className="min-w-0">
          {shares.map((s) => (
            <div key={s.path}>
              <span className="font-medium">Can't open {s.path}:</span> {s.error}. Nothing on it will play or scan until then.
            </div>
          ))}
          <Link to="/settings/server" className="underline">
            Fix it in Settings → Server → Media locations
          </Link>
        </div>
      </div>
    </div>
  );
}
