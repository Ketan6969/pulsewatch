import {
  Bell,
  ChevronsLeft,
  ChevronsRight,
  Globe,
  LayoutDashboard,
  LineChart,
  Network,
  Server,
  Settings,
} from "lucide-react";
import { NavLink } from "react-router-dom";
import { useAlerts } from "../api/queries";
import { Button } from "../components/ui/primitives";

const NAV = [
  { section: "Monitor" },
  { to: "/", label: "Overview", icon: LayoutDashboard, end: true },
  { to: "/infrastructure", label: "Infrastructure", icon: Network },
  { to: "/hosts", label: "Hosts", icon: Server },
  { to: "/monitors", label: "HTTP Monitors", icon: Globe },
  { section: "Analyze" },
  { to: "/alerts", label: "Alerts", icon: Bell, badge: true },
  { to: "/metrics", label: "Metrics", icon: LineChart },
] as const;

function Logo() {
  return (
    <svg className="sidebar__logo" viewBox="0 0 32 32" aria-hidden>
      <rect width="32" height="32" rx="6" fill="var(--color-primary)" />
      <path d="M5 17h5l3-7 5 13 3-6h6" fill="none" stroke="#fff" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function Sidebar({ collapsed, onToggle, onNavigate }: { collapsed: boolean; onToggle: () => void; onNavigate: () => void }) {
  const firing = useAlerts("firing").data?.length ?? 0;
  return (
    <aside className="sidebar" aria-label="Primary">
      <div className="sidebar__brand">
        <Logo />
        <span className="sidebar__label">Pulsewatch</span>
      </div>
      <nav className="sidebar__nav">
        {NAV.map((item, i) =>
          "section" in item ? (
            <div key={i} className="sidebar__section">
              {item.section}
            </div>
          ) : (
            <NavLink
              key={item.to}
              to={item.to}
              end={"end" in item}
              className="nav-link"
              title={collapsed ? item.label : undefined}
              onClick={onNavigate}
            >
              <item.icon size={17} strokeWidth={1.75} aria-hidden />
              <span className="sidebar__label">{item.label}</span>
              {"badge" in item && firing > 0 && (
                <span className="nav-link__badge num" aria-label={`${firing} firing alerts`}>
                  {firing}
                </span>
              )}
            </NavLink>
          ),
        )}
      </nav>
      <div className="sidebar__footer">
        <NavLink to="/settings" className="nav-link" title={collapsed ? "Settings" : undefined} onClick={onNavigate}>
          <Settings size={17} strokeWidth={1.75} aria-hidden />
          <span className="sidebar__label">Settings</span>
        </NavLink>
        <div className="sidebar__user">
          <span className="avatar" aria-hidden>
            OP
          </span>
          <span className="sidebar__label" style={{ flex: 1, minWidth: 0 }}>
            <div className="truncate" style={{ fontWeight: 500 }}>
              Operator
            </div>
            <div className="text-muted truncate" style={{ fontSize: "var(--text-xs)" }}>
              Administrator
            </div>
          </span>
          <Button
            variant="ghost"
            icon
            size="sm"
            className="sidebar__label desktop-only"
            onClick={onToggle}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {collapsed ? <ChevronsRight size={15} /> : <ChevronsLeft size={15} />}
          </Button>
        </div>
        {collapsed && (
          <Button variant="ghost" icon onClick={onToggle} aria-label="Expand sidebar" className="desktop-only" style={{ alignSelf: "center" }}>
            <ChevronsRight size={15} />
          </Button>
        )}
      </div>
    </aside>
  );
}
