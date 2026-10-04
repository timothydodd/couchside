/** What to show for something caught: an Error's message, or the value as text. */
export const errText = (e: unknown): string => (e instanceof Error ? e.message : String(e));
