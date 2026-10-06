import { create } from "zustand";

/**
 * A play queue: the files "Play all" lined up, played one after another by
 * the player. It lives for the session only. Playing something else (a
 * poster, a Continue Watching card) drops it.
 */
interface QueueState {
  fileIds: number[];
  start: (fileIds: number[]) => void;
  /** The file after fileId in the queue, or null when fileId isn't queued or is last. */
  after: (fileId: number) => number | null;
  clear: () => void;
}

export const useQueue = create<QueueState>((set, get) => ({
  fileIds: [],
  start: (fileIds) => set({ fileIds }),
  after: (fileId) => {
    const q = get().fileIds;
    const i = q.indexOf(fileId);
    return i >= 0 && i + 1 < q.length ? q[i + 1] : null;
  },
  clear: () => set({ fileIds: [] }),
}));
