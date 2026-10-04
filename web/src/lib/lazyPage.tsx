import { lazy, type ComponentType } from "react";
import { EmptyState } from "../components/ui";

/**
 * A page loaded when it's first shown, so the first download only carries
 * what everyone needs. If its file can't be fetched (the connection dropped,
 * or the server was updated and this tab still names the old build's files),
 * the page says so with a Reload button instead of a blank screen.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function lazyPage<P = any>(load: () => Promise<{ default: ComponentType<P> }>) {
  return lazy(() =>
    load().catch(() => ({
      default: (() => (
        <EmptyState title="Couldn't load this page">
          Check the connection. If Couchside was just updated, reloading picks up the new version.
          <div className="mt-3">
            <button className="btn-primary" onClick={() => location.reload()}>
              Reload
            </button>
          </div>
        </EmptyState>
      )) as ComponentType<P>,
    })),
  );
}
