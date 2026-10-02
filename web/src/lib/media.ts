import { useSyncExternalStore } from "react";

/** Phone widths: below Tailwind's md breakpoint, where the mobile layout applies. */
const PHONE = "(max-width: 47.99rem)";

/** True while the window matches the CSS media query. */
export function useMedia(query: string): boolean {
  return useSyncExternalStore(
    (cb) => {
      const m = window.matchMedia(query);
      m.addEventListener("change", cb);
      return () => m.removeEventListener("change", cb);
    },
    () => window.matchMedia(query).matches,
  );
}

export const usePhone = () => useMedia(PHONE);
