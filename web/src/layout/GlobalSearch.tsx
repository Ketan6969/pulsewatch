// Global search across hosts and monitors (combobox pattern, keyboard
// navigable, "/" to focus).
import { Globe, Search, Server } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useHosts, useMonitors } from "../api/queries";
import { matches } from "../components/ui/FilterBar";
import { HealthIndicator } from "../components/ui/status";

interface Result {
  id: string;
  group: "Hosts" | "HTTP Monitors";
  title: string;
  subtitle: string;
  status: string;
  to: string;
}

export function GlobalSearch() {
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listId = useId();
  const navigate = useNavigate();
  const hosts = useHosts().data;
  const monitors = useMonitors().data;

  const results = useMemo<Result[]>(() => {
    if (!q.trim()) return [];
    const h = (hosts ?? [])
      .filter((x) => matches(q, x.name, x.hostname, x.platform))
      .slice(0, 6)
      .map<Result>((x) => ({ id: x.id, group: "Hosts", title: x.name, subtitle: x.hostname, status: x.status, to: `/hosts/${x.id}` }));
    const m = (monitors ?? [])
      .filter((x) => matches(q, x.name, x.url))
      .slice(0, 6)
      .map<Result>((x) => ({ id: x.id, group: "HTTP Monitors", title: x.name, subtitle: x.url, status: x.status, to: `/monitors/${x.id}` }));
    return [...h, ...m];
  }, [q, hosts, monitors]);

  useEffect(() => setActive(0), [q]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (e.key === "/" && !["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName)) {
        e.preventDefault();
        inputRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const go = (r: Result) => {
    navigate(r.to);
    setQ("");
    setOpen(false);
    inputRef.current?.blur();
  };

  const showList = open && q.trim() !== "";
  return (
    <div className="search hide-mobile">
      <Search size={14} />
      <input
        ref={inputRef}
        className="input"
        type="text"
        role="combobox"
        aria-label="Search hosts and monitors"
        aria-expanded={showList}
        aria-controls={listId}
        aria-activedescendant={showList && results[active] ? `${listId}-${active}` : undefined}
        placeholder="Search hosts, monitors…"
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 120)}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setActive((a) => Math.min(a + 1, results.length - 1));
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setActive((a) => Math.max(a - 1, 0));
          } else if (e.key === "Enter" && results[active]) {
            go(results[active]);
          } else if (e.key === "Escape") {
            setQ("");
            inputRef.current?.blur();
          }
        }}
      />
      {!q && <kbd className="search__kbd">/</kbd>}
      {showList && (
        <ul className="search__results" id={listId} role="listbox">
          {results.length === 0 && <li className="search__group">No matches</li>}
          {results.map((r, i) => (
            <li key={`${r.group}-${r.id}`} role="presentation">
              {(i === 0 || results[i - 1].group !== r.group) && <div className="search__group">{r.group}</div>}
              <div
                id={`${listId}-${i}`}
                role="option"
                aria-selected={i === active}
                className="search__option"
                onMouseDown={(e) => e.preventDefault()}
                onMouseEnter={() => setActive(i)}
                onClick={() => go(r)}
              >
                {r.group === "Hosts" ? <Server size={14} className="text-muted" /> : <Globe size={14} className="text-muted" />}
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="truncate">{r.title}</div>
                  <div className="truncate text-muted mono" style={{ fontSize: "var(--text-xs)" }}>
                    {r.subtitle}
                  </div>
                </div>
                <HealthIndicator status={r.status} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
