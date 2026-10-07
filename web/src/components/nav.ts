import { Activity, Clapperboard, FolderOpen, Home, RadioTower, Settings, Tv, type LucideIcon } from "lucide-react";
import type { Route } from "../stores/router";

export type NavName = Route["name"];

export interface NavItem {
  name: NavName;
  to: string;
  label: string;
  /** A shorter label for the phone's bottom tabs. */
  short?: string;
  Icon: LucideIcon;
  admin?: boolean;
}

/** The main sections: the sidebar's first group and the phone's bottom tabs. */
export const NAV_MAIN: NavItem[] = [
  { name: "home", to: "/", label: "Home", Icon: Home },
  { name: "movies", to: "/movies", label: "Movies", Icon: Clapperboard },
  { name: "tv", to: "/tv", label: "TV shows", short: "TV", Icon: Tv },
];

/** Shown after the main sections when Live TV is configured. */
export const NAV_LIVE: NavItem = { name: "livetv", to: "/livetv", label: "Live TV", Icon: RadioTower };

/** The sidebar's second group and the phone's More sheet; users see only Settings. */
export const NAV_MANAGE: NavItem[] = [
  { name: "activity", to: "/activity", label: "Activity", Icon: Activity, admin: true },
  { name: "libraries", to: "/libraries", label: "Libraries", Icon: FolderOpen, admin: true },
  { name: "settings", to: "/settings", label: "Settings", Icon: Settings },
];

/** Which section a route belongs to, for highlighting it. */
export function sectionOf(route: Route): NavName {
  if (route.name === "item" || route.name === "episode" || route.name === "play" || route.name === "person") return "home";
  if (route.name === "watch" || route.name === "recording") return "livetv";
  if (route.name === "manage") return "libraries";
  return route.name;
}
