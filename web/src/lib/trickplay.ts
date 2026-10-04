/** A file's seek-bar preview thumbnails, from GET /api/files/{id}/trickplay. */
export interface TrickIndex {
  interval: number; // seconds between frames
  width: number; // one frame, in pixels
  height: number;
  cols: number; // frames across a sheet
  rows: number;
  count: number;
  sheets: number;
  size: number;
  mtime: number;
}

/** One frame, as a piece of a sheet image, drawn `width` pixels wide. */
export interface PreviewFrame {
  url: string;
  width: number;
  height: number;
  /** CSS background-size and background-position for the sheet. */
  size: string;
  position: string;
}

const SHOWN = 208; // px wide on screen

/** The frame nearest time t (seconds into the file), or null when there's none. */
export function previewFrame(fileId: number, ix: TrickIndex | undefined, t: number): PreviewFrame | null {
  if (!ix || ix.count === 0 || t < 0) return null;
  const n = Math.min(ix.count - 1, Math.round(t / ix.interval));
  const per = ix.cols * ix.rows;
  const tile = n % per;
  const k = SHOWN / ix.width;
  const h = Math.round(ix.height * k);
  return {
    // mtime in the URL: a changed file gets new thumbnails, and the browser may keep these.
    url: `/api/files/${fileId}/trickplay/${Math.floor(n / per)}.jpg?v=${ix.mtime}`,
    width: SHOWN,
    height: h,
    size: `${SHOWN * ix.cols}px ${h * ix.rows}px`,
    position: `-${(tile % ix.cols) * SHOWN}px -${Math.floor(tile / ix.cols) * h}px`,
  };
}
