// Where to go after signing in: the page someone opened while signed out (a
// shared link to a title, say), kept for this tab only.

const KEY = "couchside.returnTo";

/** A path on this site, or "/": never another host ("//evil.example") or a URL. */
export function safePath(p: string | null | undefined): string {
  return p && p.startsWith("/") && !p.startsWith("//") && !/[\\\s]/.test(p) ? p : "/";
}

/** Remembers the current page to come back to, unless it's a sign-in page or Home. */
export function rememberReturnTo() {
  const here = location.pathname + location.search;
  if (here === "/" || here === "/admin" || here === "/profiles" || here.startsWith("/link")) return;
  try {
    sessionStorage.setItem(KEY, here);
  } catch {
    // Storage blocked (private mode): Home it is.
  }
}

/** The remembered page (forgetting it), or "/". */
export function takeReturnTo(): string {
  let p: string | null = null;
  try {
    p = sessionStorage.getItem(KEY);
    sessionStorage.removeItem(KEY);
  } catch {
    // ignore
  }
  return safePath(p);
}

/** Forgets any remembered page (signing out). */
export function forgetReturnTo() {
  try {
    sessionStorage.removeItem(KEY);
  } catch {
    // ignore
  }
}
