import { useState } from "react";
import { Star } from "lucide-react";
import { api } from "../../lib/api";
import type { TvChannel } from "../../lib/types";
import { WEAK_SIGNAL } from "./filters";
import { notify } from "../../lib/notices";
import { errText } from "../../lib/errors";

/** The red dot for a program that will record (pulsing while it is). */
export function RecDot({ status, className = "" }: { status?: string; className?: string }) {
  if (status !== "recording" && status !== "scheduled") return null;
  return <span className={`rec-dot ${status === "recording" ? "animate-pulse" : ""} ${className}`} />;
}

/** Four bars from the tuner's signal-quality reading; amber when marginal. */
export function SignalBars({ c }: { c: TvChannel }) {
  const q = c.signalQuality;
  if (q == null) return null;
  const bars = q >= 90 ? 4 : q >= 75 ? 3 : q >= WEAK_SIGNAL ? 2 : 1;
  const weak = q < WEAK_SIGNAL;
  return (
    <span
      className="inline-flex h-2.5 items-end gap-px"
      title={`Signal quality ${q}%${c.signalStrength != null ? `, strength ${c.signalStrength}%` : ""} (at the tuner's last scan)${weak ? ": may break up" : ""}`}
      aria-label={`Signal quality ${q} percent`}
    >
      {[1, 2, 3, 4].map((i) => (
        <span
          key={i}
          className={`w-[3px] rounded-sm ${i <= bars ? (weak ? "bg-warning" : "bg-good") : "bg-border"}`}
          style={{ height: `${25 * i}%` }}
        />
      ))}
    </span>
  );
}

/** Star that pins a channel to the top of the guide. */
export function PinButton({ c, onChange, className = "" }: { c: TvChannel; onChange: () => void; className?: string }) {
  const [busy, setBusy] = useState(false);
  const toggle = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setBusy(true);
    try {
      await api(`/api/livetv/channels/${c.number}/pin`, { method: "PUT", json: { pinned: !c.pinned } });
      onChange();
    } catch (e) {
      notify(`Couldn't change favourites: ${errText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  return (
    <button
      onClick={(e) => void toggle(e)}
      disabled={busy}
      className={`rounded p-1 transition-colors ${c.pinned ? "text-warning" : "text-content-muted opacity-0 hover:text-content group-hover:opacity-100 focus-visible:opacity-100"} ${className}`}
      title={c.pinned ? "Unpin from the top" : "Pin to the top"}
      aria-label={c.pinned ? `Unpin ${c.name}` : `Pin ${c.name}`}
      aria-pressed={c.pinned}
    >
      <Star size={14} className={c.pinned ? "fill-current" : ""} />
    </button>
  );
}
