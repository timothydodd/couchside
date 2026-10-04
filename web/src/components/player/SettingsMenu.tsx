import { useEffect, useRef, useState, type ReactNode } from "react";
import { menuKeys } from "../../lib/dialog";
import { Check, ChevronLeft, ChevronRight } from "lucide-react";

export interface SettingOption {
  id: string;
  label: string;
  detail?: string;
  active?: boolean;
}

/** One row of the settings menu: a list of options, or custom content (e.g. playback info). */
export interface SettingSection {
  id: string;
  label: string;
  value?: string;
  options?: SettingOption[];
  onSelect?: (id: string) => void;
  content?: ReactNode;
  hidden?: boolean;
}

/** Plex-style settings popover: a main list, each row opening its own page. */
export default function SettingsMenu({ sections, onClose }: { sections: SettingSection[]; onClose: () => void }) {
  const [page, setPage] = useState<string | null>(null);
  const visible = sections.filter((s) => !s.hidden);
  const current = visible.find((s) => s.id === page);
  // Focus follows the page shown, so the arrow keys work from the first press.
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => {
    menu.current?.querySelector<HTMLElement>("button")?.focus();
  }, [page]);

  return (
    <div
      className="card absolute bottom-16 right-3 z-30 max-h-[70vh] w-80 overflow-y-auto p-1.5 text-sm shadow-[var(--shadow-md)]"
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
      onKeyDown={menuKeys}
      ref={menu}
      role="menu"
    >
      {!current ? (
        visible.map((s) => (
          <button
            key={s.id}
            role="menuitem"
            className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-content hover:bg-muted"
            onClick={() => setPage(s.id)}
          >
            <span className="flex-1">{s.label}</span>
            {s.value && <span className="max-w-40 truncate text-xs text-content-muted">{s.value}</span>}
            <ChevronRight size={14} className="text-content-muted" />
          </button>
        ))
      ) : (
        <>
          <button className="flex w-full items-center gap-1.5 rounded px-2 py-2 text-left text-content-secondary hover:bg-muted hover:text-content" onClick={() => setPage(null)}>
            <ChevronLeft size={15} />
            <span className="card-title !text-content-secondary">{current.label}</span>
          </button>
          <div className="my-1 border-t border-border-light" />
          {current.content ??
            current.options?.map((o) => (
              <button
                key={o.id}
                role="menuitemradio"
                aria-checked={!!o.active}
                className="flex w-full items-start gap-2 rounded px-3 py-2 text-left hover:bg-muted"
                onClick={() => {
                  current.onSelect?.(o.id);
                  setPage(null);
                  onClose();
                }}
              >
                <Check size={14} className={`mt-0.5 shrink-0 ${o.active ? "text-accent" : "invisible"}`} />
                <span className="min-w-0 flex-1">
                  <span className="block text-content">{o.label}</span>
                  {o.detail && <span className="block text-xs text-content-muted">{o.detail}</span>}
                </span>
              </button>
            ))}
        </>
      )}
    </div>
  );
}

/** Label/value rows for the playback info page. */
export function InfoRows({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <dl className="grid grid-cols-[110px_1fr] gap-x-3 gap-y-1.5 px-3 pb-2 pt-1 text-xs">
      {rows.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-content-muted">{k}</dt>
          <dd className="break-words text-content">{v}</dd>
        </div>
      ))}
    </dl>
  );
}
