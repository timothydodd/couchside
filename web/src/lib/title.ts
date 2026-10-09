import { useEffect } from "react";

/** Sets the tab's title to "<title> · Couchside" ("Couchside" while title is empty, e.g. still loading). */
export function useTitle(title: string | null | undefined) {
  useEffect(() => {
    document.title = title ? `${title} · Couchside` : "Couchside";
  }, [title]);
}
