import { create } from "zustand";
import { api } from "../lib/api";
import type { Status } from "../lib/types";
import { errText } from "../lib/errors";

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
      set({ error: errText(e) });
    }
  },
}));

// Poll faster while jobs are running so the status bar and badges feel live.
async function poll() {
  if (!polling) return;
  await useStatus.getState().refresh();
  const j = useStatus.getState().status?.jobs;
  const busy = !!j && j.queued + j.running > 0;
  if (polling) timer = setTimeout(poll, document.visibilityState === "visible" ? (busy ? 2000 : 8000) : 30000);
}
let polling = false;
let timer: ReturnType<typeof setTimeout> | undefined;
/** Start polling once there's someone signed in to poll for. */
export function startStatus() {
  if (polling) return;
  polling = true;
  void poll();
}
/** Stop polling: nobody's signed in any more. */
export function stopStatus() {
  polling = false;
  clearTimeout(timer);
}
