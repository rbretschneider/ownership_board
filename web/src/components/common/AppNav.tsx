import { ChartColumn, LayoutGrid, Settings, Waypoints, type LucideIcon } from "lucide-react";
import { BOARD_HASH, FLOW_HASH, SETTINGS_HASH, STATS_HASH, type RouteName } from "../../lib/hashRoute";

/** The top-level screens the nav links to. */
export type AppPage = Extract<RouteName, "board" | "flow" | "stats" | "settings">;

const PAGES: { page: AppPage; label: string; href: string; Icon: LucideIcon }[] = [
  { page: "board", label: "Board", href: BOARD_HASH, Icon: LayoutGrid },
  { page: "flow", label: "Flow", href: FLOW_HASH, Icon: Waypoints },
  { page: "stats", label: "Stats", href: STATS_HASH, Icon: ChartColumn },
  { page: "settings", label: "Settings", href: SETTINGS_HASH, Icon: Settings },
];

/** Props for {@link AppNav}. */
export interface AppNavProps {
  current: AppPage;
  /** Icon-only links, for the crowded board top bar. */
  isCompact?: boolean;
}

/** Links between the board, stats and settings; marks the current page. */
export function AppNav({ current, isCompact = false }: AppNavProps) {
  return (
    <nav className={`app-nav${isCompact ? " app-nav--compact" : ""}`} aria-label="Pages">
      {PAGES.filter((p) => !isCompact || p.page !== current).map(({ page, label, href, Icon }) => (
        <a
          key={page}
          href={href}
          className={`app-nav__link app-nav__link--${page}${page === current ? " app-nav__link--current" : ""}`}
          aria-current={page === current ? "page" : undefined}
          aria-label={isCompact ? label : undefined}
          title={isCompact ? label : undefined}
        >
          <Icon size={isCompact ? 20 : 18} aria-hidden="true" />
          {!isCompact && <span>{label}</span>}
        </a>
      ))}
    </nav>
  );
}
