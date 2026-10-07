import { useCallback, useState, type ImgHTMLAttributes } from "react";

/**
 * An image that fades in once it has loaded, so artwork arriving over the
 * network doesn't pop. One already in the browser's cache is complete when
 * it mounts and shows at once, so a page you come back to doesn't flicker.
 */
export default function FadeImg({ className = "", ...img }: ImgHTMLAttributes<HTMLImageElement>) {
  const [loaded, setLoaded] = useState(false);
  const ref = useCallback((el: HTMLImageElement | null) => {
    if (el?.complete && el.naturalWidth > 0) setLoaded(true);
  }, []);
  return <img ref={ref} {...img} onLoad={(e) => { setLoaded(true); img.onLoad?.(e); }} className={`img-fade ${loaded ? "img-fade-in" : ""} ${className}`} />;
}
