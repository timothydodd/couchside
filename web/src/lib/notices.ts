import { create } from "zustand";

/** A message that floats over the page for a few seconds (Notices). */
export interface Notice {
  id: number;
  text: string;
}

interface NoticeState {
  list: Notice[];
  dismiss: (id: number) => void;
}

let next = 1;

export const useNotices = create<NoticeState>((set) => ({
  list: [],
  dismiss: (id) => set((s) => ({ list: s.list.filter((n) => n.id !== id) })),
}));

/** Shows text for 8 seconds (or until dismissed). */
export function notify(text: string) {
  const id = next++;
  useNotices.setState((s) => ({ list: [...s.list.slice(-3), { id, text }] }));
  setTimeout(() => useNotices.getState().dismiss(id), 8000);
}

/**
 * Wraps a button's action so a failure (a 403, a 500, the network) shows the
 * server's message as "what: message" instead of vanishing.
 */
export function attempt<A extends unknown[]>(what: string, fn: (...args: A) => Promise<unknown>) {
  return async (...args: A) => {
    try {
      await fn(...args);
    } catch (e) {
      notify(`${what}: ${e instanceof Error ? e.message : String(e)}`);
    }
  };
}
