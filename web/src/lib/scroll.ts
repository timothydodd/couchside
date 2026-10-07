import { useCallback, useEffect, useState, type RefObject } from "react";

/**
 * Which ends of a horizontal scroller have more to show, and a way to page
 * it. Sets `data-fade` ("left", "right" or "both") on the element so
 * `.row-scroll` can fade the cut-off edge, and gives the arrow buttons
 * their disabled state. Tracks scrolling, resizes and content changes.
 */
export function useScrollEdges(ref: RefObject<HTMLElement | null>) {
  const [edges, setEdges] = useState({ left: false, right: false });

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () => {
      const left = el.scrollLeft > 1;
      const right = el.scrollLeft + el.clientWidth < el.scrollWidth - 1;
      el.dataset.fade = left && right ? "both" : left ? "left" : right ? "right" : "";
      setEdges((e) => (e.left === left && e.right === right ? e : { left, right }));
    };
    update();
    el.addEventListener("scroll", update, { passive: true });
    const ro = new ResizeObserver(update);
    ro.observe(el);
    const mo = new MutationObserver(update);
    mo.observe(el, { childList: true, subtree: true });
    return () => {
      el.removeEventListener("scroll", update);
      ro.disconnect();
      mo.disconnect();
    };
  }, [ref]);

  /** Scrolls by most of the visible width, so the last card seen stays as a landmark. */
  const page = useCallback(
    (dir: -1 | 1) => {
      const el = ref.current;
      if (!el) return;
      el.scrollBy({ left: dir * Math.max(el.clientWidth - 120, el.clientWidth * 0.6), behavior: "smooth" });
    },
    [ref],
  );

  return { ...edges, page };
}
