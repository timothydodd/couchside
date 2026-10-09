import { rangeDetail, rangeLabel } from "../lib/quality";

/** A video's dynamic range as a chip: "Dolby Vision", "HDR10" or "HLG"; nothing for SDR. */
export default function RangeChip({ range, dvProfile = 0 }: { range: string | undefined; dvProfile?: number }) {
  const label = rangeLabel(range);
  if (!label) return null;
  return (
    <span className="chip chip-hdr" title={rangeDetail(range, dvProfile) || undefined}>
      {label}
    </span>
  );
}
