/**
 * What to show for something caught: an Error's message, or the value as text.
 * A fetch that never reached the server (offline, server down) fails with a
 * TypeError whose wording differs by browser ("Failed to fetch", "Load
 * failed", "NetworkError…"); it reads the same everywhere instead.
 */
export const errText = (e: unknown): string => {
  if (e instanceof TypeError && /fetch|network|load failed/i.test(e.message))
    return "Can't reach the server. Check the connection and that Couchside is running.";
  return e instanceof Error ? e.message : String(e);
};
