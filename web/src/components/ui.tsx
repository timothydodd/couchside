import type { InputHTMLAttributes, ReactNode, Ref } from "react";
import { Loader2, Search } from "lucide-react";

// Shared primitives, ported from Portside Lite's components/ui.tsx so both
// apps look and behave the same.

export type Tone = "good" | "warning" | "critical" | "info" | "muted";

export function StatusPill({ label, tone, pulse }: { label: string; tone: Tone; pulse?: boolean }) {
  const dot = {
    good: "bg-good",
    critical: "bg-critical",
    warning: "bg-warning",
    info: "bg-info",
    muted: "bg-content-muted",
  }[tone];
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-content-secondary">
      <span className={`h-2 w-2 rounded-full ${dot} ${pulse ? "animate-pulse" : ""}`} />
      {label}
    </span>
  );
}

/** Progress meter: accent fill on a lighter track of the same hue. */
export function Meter({ value, className = "" }: { value: number; className?: string }) {
  const v = Math.max(0, Math.min(100, value));
  return (
    <div
      className={`h-1.5 w-full overflow-hidden rounded-full ${className}`}
      style={{ backgroundColor: "color-mix(in srgb, var(--accent) 18%, transparent)" }}
      role="meter"
      aria-valuenow={Math.round(v)}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <div className="h-full rounded-full bg-accent transition-[width] duration-500" style={{ width: `${v}%` }} />
    </div>
  );
}

export function StatTile({
  label,
  value,
  sub,
  tone,
  meter,
  onClick,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  tone?: "critical" | "warning" | "good";
  /** 0–100: draws a fill bar under the value. */
  meter?: number;
  onClick?: () => void;
}) {
  const ring = tone === "critical" ? "border-critical/50" : tone === "warning" ? "border-warning/50" : "border-border-light";
  const Tag = onClick ? "button" : "div";
  return (
    <Tag
      onClick={onClick}
      className={`card flex flex-col items-start gap-1 border px-4 py-3 text-left ${ring} ${onClick ? "transition-colors hover:border-accent" : ""}`}
    >
      <span className="text-xs text-content-muted">{label}</span>
      <span className="text-2xl font-semibold tabular-nums text-content">{value}</span>
      {meter !== undefined && <Meter value={meter} className="my-0.5" />}
      {sub && <span className="text-xs text-content-secondary">{sub}</span>}
    </Tag>
  );
}

export function PageHeader({ title, subtitle, children }: { title: string; subtitle?: ReactNode; children?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3 border-b border-border-light px-6 pb-3 pt-5">
      <div>
        <h1 className="text-lg font-semibold text-content">{title}</h1>
        {subtitle && <p className="mt-0.5 text-xs text-content-muted">{subtitle}</p>}
      </div>
      {children && <div className="flex flex-wrap items-center gap-2">{children}</div>}
    </div>
  );
}

export function EmptyState({ icon, title, children }: { icon?: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-6 py-16 text-center">
      {icon && <div className="text-content-muted">{icon}</div>}
      <div className="text-sm font-medium text-content">{title}</div>
      {children && <div className="max-w-md text-xs text-content-muted">{children}</div>}
    </div>
  );
}

/** A row of mutually exclusive choices (theme, quality…). */
export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { id: T; label: string }[];
  onChange: (v: T) => void;
  label: string;
}) {
  return (
    <div className="inline-flex rounded-md border border-border p-0.5" role="radiogroup" aria-label={label}>
      {options.map((o) => (
        <button
          key={o.id}
          type="button"
          role="radio"
          aria-checked={value === o.id}
          onClick={() => onChange(o.id)}
          className={`rounded px-3 py-1 text-sm transition-colors ${value === o.id ? "bg-accent text-on-accent" : "text-content-secondary hover:text-content"}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Spinner({ size = 16 }: { size?: number }) {
  return <Loader2 size={size} className="animate-spin text-content-muted" />;
}

export function ErrorNote({ children }: { children: ReactNode }) {
  return <div className="tint-critical rounded-md px-3 py-2 text-xs">{children}</div>;
}

export function SearchInput({
  value,
  onChange,
  placeholder = "Filter…",
  className = "",
  ...input
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  className?: string;
} & Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "placeholder" | "className"> & {
    ref?: Ref<HTMLInputElement>;
  }) {
  return (
    <div className={`relative ${className}`}>
      <Search size={14} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-content-muted" />
      <input {...input} className="field w-full pl-8" value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
    </div>
  );
}
