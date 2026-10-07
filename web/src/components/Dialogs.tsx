import { useEffect, useRef, useState } from "react";
import Modal from "./Modal";
import { answer, useAsk, type Ask } from "../lib/ask";

/** The confirm or prompt at the front of the queue (lib/ask.ts), over everything but notices. */
export default function Dialogs() {
  const ask = useAsk((s) => s.queue[0]);
  if (!ask) return null;
  // Keyed on the id, so a second ask starts with fresh state.
  return ask.kind === "confirm" ? <Confirm key={ask.id} ask={ask} /> : <Prompt key={ask.id} ask={ask} />;
}

function Confirm({ ask }: { ask: Extract<Ask, { kind: "confirm" }> }) {
  const focus = useRef<HTMLButtonElement>(null);
  // Enter answers straight away: the safe button for a destructive question, the action otherwise.
  useEffect(() => focus.current?.focus(), []);
  return (
    <Modal label={ask.title} role="alertdialog" zIndex="z-50" onClose={() => answer(ask.id, false)}>
      <div className="text-base font-semibold text-content">{ask.title}</div>
      {ask.body && <p className="mt-2 whitespace-pre-line text-sm text-content-secondary">{ask.body}</p>}
      <div className="mt-4 flex justify-end gap-2">
        <button type="button" className="btn-quiet" ref={ask.danger ? focus : undefined} onClick={() => answer(ask.id, false)}>
          Cancel
        </button>
        <button type="button" className={ask.danger ? "btn-danger" : "btn-primary"} ref={ask.danger ? undefined : focus} onClick={() => answer(ask.id, true)}>
          {ask.action ?? "OK"}
        </button>
      </div>
    </Modal>
  );
}

function Prompt({ ask }: { ask: Extract<Ask, { kind: "prompt" }> }) {
  const [text, setText] = useState(ask.value ?? "");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    input.current?.focus();
    input.current?.select();
  }, []);
  const ok = text.trim().length > 0;
  return (
    <Modal label={ask.title} zIndex="z-50" onClose={() => answer(ask.id, null)}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (ok) answer(ask.id, text.trim());
        }}
      >
        <label className="block">
          <span className="text-base font-semibold text-content">{ask.title}</span>
          {ask.body && <span className="mt-1 block text-sm text-content-secondary">{ask.body}</span>}
          <input
            ref={input}
            className="field mt-3 w-full"
            value={text}
            placeholder={ask.placeholder}
            maxLength={ask.maxLength ?? 200}
            onChange={(e) => setText(e.target.value)}
          />
        </label>
        <div className="mt-4 flex justify-end gap-2">
          <button type="button" className="btn-quiet" onClick={() => answer(ask.id, null)}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={!ok}>
            {ask.action ?? "Save"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
