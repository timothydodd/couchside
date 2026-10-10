import type { AnchorHTMLAttributes, MouseEvent } from "react";
import { useRouter } from "../stores/router";
import { noteClick } from "../lib/transition";

/** <a> that routes client-side but keeps middle-click / ctrl-click working. */
export default function Link({ to, onClick, ...rest }: { to: string } & AnchorHTMLAttributes<HTMLAnchorElement>) {
  const go = useRouter((s) => s.go);
  const handle = (e: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(e);
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    noteClick(e.currentTarget);
    go(to);
  };
  return <a href={to} onClick={handle} {...rest} />;
}
