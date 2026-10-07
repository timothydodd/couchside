import { create } from "zustand";

/**
 * Couchside's own confirm and prompt, in place of the browser's: they look
 * like the rest of the app, trap focus, and can be answered with Enter and
 * Escape. `components/Dialogs.tsx` draws whichever is open; these functions
 * resolve when it's answered.
 */

export interface ConfirmOptions {
  title: string;
  /** A sentence or two under the title. */
  body?: string;
  /** The confirming button's label; the default is "OK". */
  action?: string;
  /** Destructive: the confirming button is red and Cancel takes focus first. */
  danger?: boolean;
}

export interface PromptOptions {
  title: string;
  body?: string;
  /** The field's starting text. */
  value?: string;
  placeholder?: string;
  /** The confirming button's label; the default is "Save". */
  action?: string;
  maxLength?: number;
}

export type Ask =
  | ({ id: number; kind: "confirm"; resolve: (ok: boolean) => void } & ConfirmOptions)
  | ({ id: number; kind: "prompt"; resolve: (text: string | null) => void } & PromptOptions);

interface AskState {
  /** Asks waiting for an answer, oldest first; only the first is shown. */
  queue: Ask[];
}

let next = 1;

export const useAsk = create<AskState>(() => ({ queue: [] }));

function push(ask: Ask) {
  useAsk.setState((s) => ({ queue: [...s.queue, ask] }));
}

/** Answers the ask at the front of the queue and drops it. */
export function answer(id: number, value: boolean | string | null) {
  const ask = useAsk.getState().queue.find((a) => a.id === id);
  if (!ask) return;
  useAsk.setState((s) => ({ queue: s.queue.filter((a) => a.id !== id) }));
  if (ask.kind === "confirm") ask.resolve(value === true);
  else ask.resolve(typeof value === "string" ? value : null);
}

/** Asks a yes/no question; resolves true when confirmed. */
export function confirmDialog(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => push({ id: next++, kind: "confirm", resolve, ...opts }));
}

/** Asks for a line of text; resolves null when cancelled. */
export function promptDialog(opts: PromptOptions): Promise<string | null> {
  return new Promise((resolve) => push({ id: next++, kind: "prompt", resolve, ...opts }));
}
