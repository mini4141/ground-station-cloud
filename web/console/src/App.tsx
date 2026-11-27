import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { captureTokenFromHash, getToken, loadConfig, loginUrl, logout, makeClient, type Api, type Config, type SimClock } from "./api/client";
import { BillingPage } from "./pages/Billing";
import { LivePage } from "./pages/Live";
import { PassesPage } from "./pages/Passes";
import { SlaPage } from "./pages/Sla";
import { StationsPage } from "./pages/Stations";
import { usePoll } from "./usePoll";

const Ctx = createContext<Api>(null as unknown as Api);
export const useApi = () => useContext(Ctx);

const TABS = [
  ["/", "Stations"],
  ["/passes", "Passes"],
  ["/live", "Live telemetry"],
  ["/sla", "SLA"],
  ["/billing", "Billing"],
] as const;

export function App() {
  const [cfg, setCfg] = useState<Config | null>(null);
  const [path, setPath] = useState(location.pathname);
  useEffect(() => {
    captureTokenFromHash();
    loadConfig().then(setCfg);
    const onPop = () => setPath(location.pathname);
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  const api = useMemo(() => (cfg ? makeClient(cfg) : null), [cfg]);
  if (!cfg || !api) return <div className="center">Loading…</div>;

  if (!getToken()) {
    return (
      <div className="center login">
        <h1>Ground Station Cloud</h1>
        <p>Operator console: schedule passes, watch live telemetry, track SLA and billing.</p>
        <a className="btn primary" data-testid="login" href={loginUrl(cfg)}>
          Sign in {cfg.authMode === "cognito" ? "with Cognito" : "(dev OIDC)"}
        </a>
      </div>
    );
  }

  const go = (to: string) => (e: React.MouseEvent) => {
    e.preventDefault();
    history.pushState(null, "", to);
    setPath(to);
  };
  return (
    <Ctx.Provider value={api}>
      <header>
        <div className="brand">
          <span className="dot" /> Ground Station Cloud
        </div>
        <nav>
          {TABS.map(([to, label]) => (
            <a key={to} href={to} className={path === to ? "active" : ""} onClick={go(to)}>
              {label}
            </a>
          ))}
        </nav>
        <SimClockBadge />
        <button className="btn" onClick={logout}>
          Sign out
        </button>
      </header>
      <main>
        {path === "/passes" ? <PassesPage /> : path === "/live" ? <LivePage /> : path === "/sla" ? <SlaPage /> : path === "/billing" ? <BillingPage /> : <StationsPage />}
      </main>
    </Ctx.Provider>
  );
}

function SimClockBadge() {
  const api = useApi();
  const clock = usePoll<SimClock | undefined>(async () => (await api.GET("/sim/clock")).data, 2000);
  return (
    <div className="clock" data-testid="sim-clock" title="Simulated mission time (UTC)">
      {clock ? `${clock.sim_time.replace("T", " ").slice(0, 19)} UTC · ×${clock.scale}` : "…"}
    </div>
  );
}
