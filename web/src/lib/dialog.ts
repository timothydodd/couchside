import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type RefObject } from "react";

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** Open dialogs, innermost last: only the top one answers Escape and Tab. */
const open: HTMLElement[] = [];

/**
 * Makes ref a modal dialog for keyboards and screen readers: focus moves into
 * it on open, Tab stays inside it, Escape closes it, and focus goes back to
 * whatever opened it. The element should carry role="dialog" and
 * aria-modal="true". A dialog opened over another (a confirm from a panel)
 * takes the keys until it closes.
 */
export function useDialog(ref: RefObject<HTMLElement | null>, onClose: () => void) {
  const close = useRef(onClose);
  close.current = onClose;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const opener = document.activeElement as HTMLElement | null;
    const focusables = () => Array.from(el.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((f) => f.offsetParent !== null);
    if (!el.contains(document.activeElement)) {
      if (!el.hasAttribute("tabindex")) el.tabIndex = -1;
      el.focus({ preventScroll: true });
    }
    open.push(el);
    const onKey = (e: KeyboardEvent) => {
      if (open[open.length - 1] !== el) return;
      if (e.key === "Escape") {
        close.current();
        return;
      }
      if (e.key !== "Tab") return;
      const list = focusables();
      if (!list.length) {
        e.preventDefault();
        return;
      }
      const first = list[0];
      const last = list[list.length - 1];
      const at = document.activeElement;
      if (e.shiftKey && (at === first || at === el || !el.contains(at))) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (at === last || !el.contains(at))) {
        e.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      open.splice(open.lastIndexOf(el), 1);
      opener?.focus?.({ preventScroll: true });
    };
  }, [ref]);
}

/**
 * onKeyDown for a role="menu" container: the arrow keys, Home and End move
 * between its buttons. The keys stop here, so a menu inside the player
 * doesn't also seek or change the volume.
 */
export function menuKeys(e: ReactKeyboardEvent<HTMLElement>) {
  if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
  const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>("button:not([disabled])"));
  if (!items.length) return;
  e.preventDefault();
  e.stopPropagation();
  const at = items.indexOf(document.activeElement as HTMLElement);
  const down = e.key === "ArrowDown";
  const next = e.key === "Home" ? 0 : e.key === "End" ? items.length - 1 : at < 0 ? (down ? 0 : items.length - 1) : (at + (down ? 1 : -1) + items.length) % items.length;
  items[next].focus();
}

/**
 * onKeyDown for a role="radiogroup": the arrow keys move between its
 * role="radio" buttons and pick the one landed on, as native radios do.
 * Pair with tabIndex 0 on the checked button and -1 on the rest.
 */
export function radioKeys(e: ReactKeyboardEvent<HTMLElement>) {
  if (!["ArrowDown", "ArrowUp", "ArrowLeft", "ArrowRight"].includes(e.key)) return;
  const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('[role="radio"]:not([disabled])'));
  if (!items.length) return;
  e.preventDefault();
  const at = items.indexOf(document.activeElement as HTMLElement);
  const forward = e.key === "ArrowDown" || e.key === "ArrowRight";
  const next = at < 0 ? 0 : (at + (forward ? 1 : -1) + items.length) % items.length;
  items[next].focus();
  items[next].click();
}
