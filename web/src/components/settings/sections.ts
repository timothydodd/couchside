import { Database, Gauge, Radio, SlidersHorizontal, SquareTerminal, UserRound, Users, type LucideIcon } from "lucide-react";
import type { SettingsSection } from "../../stores/router";

/**
 * The Settings pages. "you" is /settings, every profile's own; the rest are
 * the server's and only admins see them (as sub-items under Settings in the
 * sidebar, and as tabs on phones).
 */
export const SECTIONS: { id: SettingsSection; label: string; to: string; Icon: LucideIcon; blurb: string }[] = [
  { id: "you", label: "Your settings", to: "/settings", Icon: UserRound, blurb: "Your preferences and account. Other profiles keep their own." },
  { id: "system", label: "System", to: "/settings/system", Icon: Gauge, blurb: "Load now and over time, who's connected, and how the server encodes." },
  { id: "console", label: "Console", to: "/settings/console", Icon: SquareTerminal, blurb: "The server's log as it happens." },
  { id: "accounts", label: "Accounts", to: "/settings/accounts", Icon: Users, blurb: "Who can sign in, and what they can do." },
  { id: "metadata", label: "Metadata", to: "/settings/metadata", Icon: Database, blurb: "Where titles, plots and artwork come from." },
  { id: "livetv", label: "Live TV", to: "/settings/livetv", Icon: Radio, blurb: "Tuner, guide, recordings folder and your own channels." },
  { id: "advanced", label: "Advanced", to: "/settings/advanced", Icon: SlidersHorizontal, blurb: "Recording padding, commercial-skip timing and backups." },
];
