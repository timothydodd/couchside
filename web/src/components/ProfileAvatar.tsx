import type { Profile, ProfileColor } from "../lib/types";

/** Avatar colours in the order the picker offers them, with the token each uses. */
export const PROFILE_COLORS: { id: ProfileColor; token: string }[] = [
  { id: "accent", token: "--accent" },
  { id: "pink", token: "--brand-pink" },
  { id: "cyan", token: "--brand-cyan" },
  { id: "secondary", token: "--secondary" },
  { id: "good", token: "--good" },
  { id: "warning", token: "--warning" },
  { id: "critical", token: "--critical" },
];

export const avatarStyle = (color: ProfileColor) =>
  ({ "--avatar": `var(${PROFILE_COLORS.find((c) => c.id === color)?.token ?? "--accent"})` }) as React.CSSProperties;

/** A profile's initial on its colour. size is the edge length in px. */
export default function ProfileAvatar({ profile, size = 28, className = "" }: { profile: Pick<Profile, "name" | "color">; size?: number; className?: string }) {
  return (
    <span className={`avatar ${className}`} style={{ ...avatarStyle(profile.color), width: size, height: size, fontSize: size * 0.45 }} aria-hidden>
      {Array.from(profile.name.trim())[0] ?? "?"}
    </span>
  );
}
