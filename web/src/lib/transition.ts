import { flushSync } from "react-dom";

/**
 * Page changes run as view transitions: the outgoing page slides out as the
 * new one comes in (forward into a title, back out of it, or a cross-fade
 * between sidebar sections). Where the browser has no view transitions, or
 * the system asks for less motion, the change is instant and `<main>` keeps
 * its plain fade (`.anim-page`).
 */
export const viewTransitions = typeof document !== "undefined" && "startViewTransition" in document;

export type NavKind = "forward" | "back" | "swap";

/** Runs a route change (a store update), animated when it can be. */
export function transition(update: () => void, kind: NavKind) {
  if (!viewTransitions || document.hidden || matchMedia("(prefers-reduced-motion: reduce)").matches) {
    update();
    return;
  }
  const root = document.documentElement;
  root.dataset.nav = kind;
  const t = document.startViewTransition(() => flushSync(update));
  // A skipped transition still applied the route change; there's nothing to report.
  t.ready.catch(() => {});
  void t.finished.catch(() => {}).finally(() => {
    delete root.dataset.nav;
  });
}
