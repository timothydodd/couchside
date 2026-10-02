import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import PosterCard from "./PosterCard";
import type { ItemSummary } from "../lib/types";

// Columns are derived from the container width. Phones (under md) get
// smaller cards, gaps and margins (the .gutter widths), so three fit across.
const sizes = (width: number) =>
  width < 768 ? { minCard: 100, gap: 12, pad: 16, caption: 44 } : { minCard: 168, gap: 20, pad: 24, caption: 48 };

// Scroll offsets per grid, so going back from a detail page lands where you were.
const scrollMemory = new Map<string, number>();

/** Virtualised, responsive poster grid. Renders only the visible rows. */
export default function PosterGrid({ items, memoryKey }: { items: ItemSummary[]; memoryKey: string }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setWidth(e.contentRect.width));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const { minCard: MIN_CARD, gap: GAP, pad: PAD, caption: CAPTION } = sizes(width);
  const inner = Math.max(0, width - PAD * 2);
  const cols = Math.max(2, Math.floor((inner + GAP) / (MIN_CARD + GAP)));
  const cardW = cols ? (inner - GAP * (cols - 1)) / cols : MIN_CARD;
  const rowH = cardW * 1.5 + CAPTION + GAP;
  const rows = Math.ceil(items.length / cols);

  const virtualizer = useVirtualizer({
    count: rows,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowH,
    overscan: 3,
    initialOffset: () => scrollMemory.get(memoryKey) ?? 0,
  });

  useEffect(() => {
    virtualizer.measure();
  }, [rowH, virtualizer]);

  return (
    <div
      ref={scrollRef}
      className="min-h-0 flex-1 overflow-auto"
      onScroll={(e) => scrollMemory.set(memoryKey, e.currentTarget.scrollTop)}
    >
      {width > 0 && (
        <div className="relative w-full" style={{ height: virtualizer.getTotalSize() + PAD * 2 }}>
          {virtualizer.getVirtualItems().map((row) => (
            <div
              key={row.key}
              className="absolute left-0 grid w-full"
              style={{
                top: row.start + PAD,
                height: rowH - GAP,
                paddingInline: PAD,
                gap: GAP,
                gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
              }}
            >
              {items.slice(row.index * cols, row.index * cols + cols).map((it) => (
                <PosterCard key={it.id} item={it} />
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
