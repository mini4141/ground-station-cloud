import { useApi } from "../App";
import type { SlaRow } from "../api/client";
import { usePoll } from "../usePoll";

function Table({ title, rows, testid }: { title: string; rows: SlaRow[]; testid: string }) {
  return (
    <>
      <h3>{title}</h3>
      <table data-testid={testid}>
        <thead><tr><th>Name</th><th>Passes</th><th>Success</th><th>Success rate</th><th>Command ack p95</th><th>Data delivery p95</th></tr></thead>
        <tbody>
          {rows.length === 0 && <tr><td colSpan={6} className="muted">No completed passes yet</td></tr>}
          {rows.map((r) => (
            <tr key={r.name}>
              <td><b>{r.name}</b></td><td>{r.passes}</td><td>{r.successes}</td>
              <td><span className={`chip ${r.success_rate >= 0.95 ? "completed" : r.success_rate >= 0.8 ? "dispatched" : "failed"}`}>{(r.success_rate * 100).toFixed(0)}%</span></td>
              <td>{(r.ack_latency_p95_ms ?? 0).toFixed(0)} ms</td><td>{(r.delivery_delay_p95_ms ?? 0).toFixed(0)} ms</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

export function SlaPage() {
  const api = useApi();
  const rows = usePoll<SlaRow[]>(async () => (await api.GET("/sla")).data ?? [], 2000) ?? [];
  const st = rows.filter((r) => r.scope === "station");
  const total = st.reduce((a, r) => a + r.passes, 0);
  const ok = st.reduce((a, r) => a + r.successes, 0);
  return (
    <>
      <h2>SLA monitoring</h2>
      <p className="muted">A pass succeeds when the modem held lock for at least 80% of it.</p>
      <div className="tiles">
        <div className="tile" data-testid="sla-total"><div className="tile-label">Passes completed</div><div className="tile-value">{total}</div></div>
        <div className={`tile ${total && ok === total ? "good" : ""}`}><div className="tile-label">Pass success rate</div><div className="tile-value">{total ? `${((ok / total) * 100).toFixed(0)}%` : "–"}</div></div>
      </div>
      <Table title="Per station" rows={st} testid="sla-stations" />
      <Table title="Per satellite" rows={rows.filter((r) => r.scope === "satellite")} testid="sla-satellites" />
    </>
  );
}
