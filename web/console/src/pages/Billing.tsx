import { useApi } from "../App";
import type { BillingRow } from "../api/client";
import { usePoll } from "../usePoll";

const money = (minor: number) => `$${(minor / 100).toFixed(2)}`;

export function BillingPage() {
  const api = useApi();
  const rows = usePoll<BillingRow[]>(async () => (await api.GET("/billing")).data ?? [], 2000) ?? [];
  const minutes = rows.reduce((a, r) => a + r.minutes, 0);
  const total = rows.reduce((a, r) => a + r.amount_minor, 0);
  return (
    <>
      <h2>Billing</h2>
      <p className="muted">Minutes tracked × station rate, per month.</p>
      <div className="tiles">
        <div className="tile" data-testid="billing-minutes"><div className="tile-label">Minutes used</div><div className="tile-value">{minutes.toFixed(1)}</div></div>
        <div className="tile good" data-testid="billing-total"><div className="tile-label">Amount due</div><div className="tile-value">{money(total)}</div></div>
      </div>
      <table data-testid="billing-table">
        <thead><tr><th>Month</th><th>Station</th><th>Minutes</th><th>Rate / min</th><th>Amount</th></tr></thead>
        <tbody>
          {rows.length === 0 && <tr><td colSpan={5} className="muted">No billable passes yet</td></tr>}
          {rows.map((r) => (
            <tr key={r.month + r.station}>
              <td>{r.month}</td><td><b>{r.station}</b></td><td>{r.minutes.toFixed(1)}</td><td>{money(r.rate_per_min_minor)}</td><td>{money(r.amount_minor)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}
