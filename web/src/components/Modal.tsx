import { useRef, type ReactNode } from "react";
import { useDialog } from "../lib/dialog";

/**
 * A centred dialog over a dimmed page: focus moves in, Tab stays inside,
 * Escape and a click outside close it (unless `busy`). Pass `className` for
 * the card's width or height (default `max-w-md p-4`).
 */
export default function Modal({
  label,
  onClose,
  busy = false,
  className = "max-w-md p-4",
  role = "dialog",
  zIndex = "z-40",
  children,
}: {
  label: string;
  onClose: () => void;
  /** Ignore Escape and outside clicks while something is saving. */
  busy?: boolean;
  className?: string;
  role?: "dialog" | "alertdialog";
  /** `z-50` for a dialog that may open over a side panel (also z-40). */
  zIndex?: "z-40" | "z-50";
  children: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const close = () => !busy && onClose();
  useDialog(ref, close);
  return (
    <div className={`fixed inset-0 ${zIndex} flex items-center justify-center bg-backdrop/55 p-4`} onClick={close}>
      <div className={`card w-full shadow-[var(--shadow-md)] ${className}`} onClick={(e) => e.stopPropagation()} role={role} aria-modal="true" aria-label={label} ref={ref}>
        {children}
      </div>
    </div>
  );
}
