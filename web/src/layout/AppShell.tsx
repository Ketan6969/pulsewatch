import { useIsFetching, useQueryClient } from "@tanstack/react-query";
import { Menu, Moon, RefreshCw, Sun } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { Link, Outlet, useLocation } from "react-router-dom";
import { Button } from "../components/ui/primitives";
import { TimeRangeSelector } from "../components/ui/TimeRangeSelector";
import { useTheme } from "../lib/theme";
import { GlobalSearch } from "./GlobalSearch";
import { Sidebar } from "./Sidebar";

export interface Crumb {
  label: string;
  to?: string;
}

interface PageMeta {
  title: string;
  crumbs?: Crumb[];
  /** Hide the time range selector on pages where it has no effect. */
  noTimeRange?: boolean;
}

const PageMetaContext = createContext<(m: PageMeta) => void>(() => {});

/** Pages call this to set the top bar title and breadcrumbs. */
export function usePageMeta(meta: PageMeta) {
  const set = useContext(PageMetaContext);
  const key = JSON.stringify(meta);
  useEffect(() => {
    set(meta);
    document.title = `${meta.title} · Pulsewatch`;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, set]);
}

const COLLAPSE_KEY = "pulsewatch.sidebarCollapsed";

export function AppShell() {
  const [meta, setMeta] = useState<PageMeta>({ title: "" });
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(COLLAPSE_KEY) === "1";
    } catch {
      return false;
    }
  });
  const [mobileOpen, setMobileOpen] = useState(false);
  const { theme, toggle } = useTheme();
  const qc = useQueryClient();
  const fetching = useIsFetching();
  const location = useLocation();

  useEffect(() => setMobileOpen(false), [location.pathname]);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((c) => {
      try {
        localStorage.setItem(COLLAPSE_KEY, c ? "0" : "1");
      } catch {
        /* ignore */
      }
      return !c;
    });
  }, []);

  return (
    <PageMetaContext.Provider value={setMeta}>
      <a href="#main" className="skip-link">
        Skip to content
      </a>
      <div className="shell" data-collapsed={collapsed} data-mobile-open={mobileOpen}>
        <Sidebar collapsed={collapsed} onToggle={toggleCollapsed} onNavigate={() => setMobileOpen(false)} />
        {mobileOpen && <div className="scrim" onClick={() => setMobileOpen(false)} />}
        <header className="topbar">
          <Button variant="ghost" icon className="mobile-only" aria-label="Open navigation" onClick={() => setMobileOpen(true)}>
            <Menu size={18} />
          </Button>
          <div className="topbar__title">
            {meta.crumbs && meta.crumbs.length > 0 && (
              <nav aria-label="Breadcrumb" className="breadcrumbs">
                {meta.crumbs.map((c, i) => (
                  <span key={i}>
                    {c.to ? <Link to={c.to}>{c.label}</Link> : c.label}
                    {i < meta.crumbs!.length - 1 && <span aria-hidden> / </span>}
                  </span>
                ))}
              </nav>
            )}
            <h1 className="truncate">{meta.title}</h1>
          </div>
          <div className="topbar__actions">
            <GlobalSearch />
            {!meta.noTimeRange && (
              <span className="hide-mobile">
                <TimeRangeSelector />
              </span>
            )}
            <Button
              icon
              onClick={() => qc.invalidateQueries()}
              aria-label="Refresh data"
              title="Refresh (auto-refresh every 15s)"
            >
              <RefreshCw size={15} className={fetching ? "spin" : undefined} />
            </Button>
            <Button icon onClick={toggle} aria-label={`Switch to ${theme === "light" ? "dark" : "light"} theme`} title="Toggle theme">
              {theme === "light" ? <Moon size={15} /> : <Sun size={15} />}
            </Button>
          </div>
        </header>
        <main className="main" id="main" tabIndex={-1}>
          {!meta.noTimeRange && (
            <div className="mobile-only" style={{ marginBottom: 12 }}>
              <TimeRangeSelector />
            </div>
          )}
          <Outlet />
        </main>
      </div>
    </PageMetaContext.Provider>
  );
}

export function Page({ children }: { children: ReactNode }) {
  return <div className="page">{children}</div>;
}
