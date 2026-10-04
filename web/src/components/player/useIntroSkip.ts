import { useEffect, useRef, type RefObject } from "react";
import type { BreakMode, MarkedSegment } from "../../lib/types";

const END_SLACK = 0.5; // the last half second of the intro counts as over

/**
 * The intro and end credits of what's playing. While the playhead is in the
 * intro, `intro.skip` jumps past it (offered as a button; in "auto" mode it
 * happens by itself, once, so seeking back into the intro lets it play).
 * `inCredits` is true from where the credits start.
 */
export function useIntroSkip(videoRef: RefObject<HTMLVideoElement | null>, time: number, segments: MarkedSegment[] | undefined, mode: BreakMode) {
  const intro = segments?.find((s) => s.kind === "intro");
  const credits = segments?.find((s) => s.kind === "credits");
  const inIntro = !!intro && mode !== "off" && time >= intro.start && time < intro.end - END_SLACK;
  const skipped = useRef<MarkedSegment | null>(null); // the intro auto-skip has dealt with

  const skip = () => {
    const v = videoRef.current;
    if (v && intro) v.currentTime = intro.end;
  };
  useEffect(() => {
    if (mode !== "auto" || !inIntro || !intro || skipped.current === intro) return;
    skipped.current = intro;
    const v = videoRef.current;
    if (v) v.currentTime = intro.end;
  }, [mode, inIntro, intro, videoRef]);

  return {
    intro: inIntro && (mode === "button" || skipped.current === intro) ? { skip } : null,
    inCredits: !!credits && time >= credits.start,
  };
}
