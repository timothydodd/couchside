import { create } from "zustand";

/** One thing lined up to play. */
export interface QueueEntry {
  fileId: number;
  title: string;
  subtitle?: string; // "S1 E4 · The One Where…", "1995", "Part 2"
}

/**
 * The play queue: what plays next, shown in the player (the queue button,
 * previous/next). Either explicit (Play all on a selection: the titles in
 * grid order) or implicit (the rest of the season of the episode playing).
 * It lives for the session; opening something that isn't in it starts over.
 */
interface QueueState {
  entries: QueueEntry[];
  /** From Play all (kept as the viewer moves through it), not built from the season. */
  explicit: boolean;
  start: (entries: QueueEntry[], explicit?: boolean) => void;
  after: (fileId: number) => number | null;
  before: (fileId: number) => number | null;
  has: (fileId: number) => boolean;
  remove: (fileId: number) => void;
  clear: () => void;
}

export const useQueue = create<QueueState>((set, get) => ({
  entries: [],
  explicit: false,
  start: (entries, explicit = true) => set({ entries, explicit }),
  after: (fileId) => {
    const q = get().entries;
    const i = q.findIndex((e) => e.fileId === fileId);
    return i >= 0 && i + 1 < q.length ? q[i + 1].fileId : null;
  },
  before: (fileId) => {
    const q = get().entries;
    const i = q.findIndex((e) => e.fileId === fileId);
    return i > 0 ? q[i - 1].fileId : null;
  },
  has: (fileId) => get().entries.some((e) => e.fileId === fileId),
  remove: (fileId) => set((s) => ({ entries: s.entries.filter((e) => e.fileId !== fileId) })),
  clear: () => set({ entries: [], explicit: false }),
}));
