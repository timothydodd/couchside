import { flushSync } from "react-dom";

/**
 * Page changes run as view transitions: the outgoing page slides out as the
 * new one comes in (forward into a title, back out of it, or a cross-fade
 * between sidebar sections), and a poster marked `data-hero` grows into the
 * same title's poster on the next page. Where the browser has no view
 * transitions, or the system asks for less motion, the change is instant and
 * `<main>` keeps its plain fade (`.anim-page`).
 */
export const viewTransitions = typeof document !== "undefined" && "startViewTransition" in document;

export type NavKind = "forward" | "back" | "swap";

// The poster the user just clicked, set by Link before it navigates.
let clicked: HTMLElement | null = null;

/** Called by Link: the poster inside the clicked link, if any, morphs into the next page. */
export function noteClick(link: HTMLElement) {
  clicked = link.querySelector<HTMLElement>("[data-hero]");
}

// How long a page change may hold the old picture while the new page's poster
// loads: into a title, its page may still be fetching; back out, the cards are
// cached, so only a frame or two for a virtualised grid to lay them out. Longer
// feels like a stall; shorter and a first visit never morphs.
const HERO_WAIT_MS = 300;
const HERO_WAIT_BACK_MS = 50;

/** Runs a route change (a store update), animated when it can be. */
export function transition(update: () => void, kind: NavKind) {
  const from = clicked ?? document.querySelector<HTMLElement>("main [data-hero-page]");
  clicked = null;
  if (!viewTransitions || document.hidden || matchMedia("(prefers-reduced-motion: reduce)").matches) {
    update();
    return;
  }
  const root = document.documentElement;
  const id = from && visible(from) ? from.dataset.hero : undefined;
  if (from && id) from.style.viewTransitionName = "hero";
  root.dataset.nav = kind;
  let to: HTMLElement | null = null;
  const t = document.startViewTransition(async () => {
    flushSync(update);
    if (from) from.style.viewTransitionName = "";
    if (!id) return;
    to = await findHero(id, from?.hasAttribute("data-hero-page") ? HERO_WAIT_BACK_MS : HERO_WAIT_MS);
    if (to) to.style.viewTransitionName = "hero";
  });
  void t.finished.finally(() => {
    if (to) to.style.viewTransitionName = "";
    delete root.dataset.nav;
  });
}

/** The same title's poster on the new page: its detail poster, else a card on screen. Waits briefly for it to load. */
async function findHero(id: string, waitMs: number): Promise<HTMLElement | null> {
  const deadline = performance.now() + waitMs;
  for (;;) {
    const sel = `main [data-hero="${CSS.escape(id)}"]`;
    const page = document.querySelector<HTMLElement>(`${sel}[data-hero-page]`);
    // The title page's poster is there but hidden (phones): nothing to wait for.
    if (page && !visible(page)) return null;
    const el = page ?? Array.from(document.querySelectorAll<HTMLElement>(sel)).find(visible) ?? null;
    if (el) {
      const img = el.querySelector("img");
      if (!img || img.complete) return el;
      const left = deadline - performance.now();
      if (left <= 0) return null;
      // Morphing into an empty frame looks worse than not morphing at all.
      const ok = await Promise.race([img.decode().then(() => true, () => false), wait(left).then(() => false)]);
      return ok ? el : null;
    }
    if (performance.now() >= deadline) return null;
    await new Promise(requestAnimationFrame);
  }
}

/** Laid out and at least partly in the window (a poster hidden on phones, or a card scrolled away, doesn't count). */
function visible(el: HTMLElement) {
  const r = el.getBoundingClientRect();
  return r.width > 0 && r.bottom > 0 && r.top < innerHeight && r.right > 0 && r.left < innerWidth;
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
