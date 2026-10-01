import { create } from "zustand";
import { api } from "../lib/api";
import type { Status } from "../lib/types";

interface StatusState {
  status: Status | null;
  error: string | null;
  refresh: () => Promise<void>;
}

export const useStatus = create<StatusState>((set) => ({
  status: null,
  error: null,
  refresh: async () => {
    try {
      set({ status: await api<Status>("/api/status"), error: null });
    } catch (e) {
      set({ error: e instanceof Error ? e.message : String(e) });
    }
  },
}));

// Poll faster while jobs are running so the status bar and badges feel live.
async function poll() {
  await useStatus.getState().refresh();
  const j = useStatus.getState().status?.jobs;
  const busy = !!j && j.queued + j.running > 0;
  setTimeout(poll, document.visibilityState === "visible" ? (busy ? 2000 : 8000) : 30000);
}
let polling = false;
/** Start polling once there's someone signed in to poll for. */
export function startStatus() {
  if (polling) return;
  polling = true;
  void poll();
}
