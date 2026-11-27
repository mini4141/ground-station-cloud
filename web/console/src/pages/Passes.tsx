import { useEffect, useState } from "react";
import { useApi } from "../App";
import type { Booking, Pass, Satellite, Station } from "../api/client";
import { usePoll } from "../usePoll";

const HOURS = 24;
const W = 1000;
const ROW = 46;
const LEFT = 90;

const fmt = (iso: string) => iso.replace("T", " ").slice(0, 19) + "Z";

export function PassesPage() {
  const api = useApi();
  const [satId, setSatId] = useState<string>();
  const [from, setFrom] = useState<string>();
  const [passes, setPasses] = useState<Pass[]>([]);
  const [selected, setSelected] = useState<Pass>();
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();
  const [tle, setTle] = useState("");

  const sats = usePoll<Satellite[]>(async () => (await api.GET("/satellites")).data ?? [], 3000);
  const stations = usePoll<Station[]>(async () => (await api.GET("/stations")).data ?? [], 5000);
  const bookings = usePoll<Booking[]>(async () => (await api.GET("/bookings")).data ?? [], 1500);
  const clock = usePoll(async () => (await api.GET("/sim/clock")).data, 1500);

  useEffect(() => {
    if (!satId && sats?.length) setSatId(sats[0].id);
  }, [sats, satId]);
  useEffect(() => {
    if (!from && clock) setFrom(clock.sim_time);
  }, [clock, from]);
  useEffect(() => {
    if (!satId || !from) return;
    const to = new Date(new Date(from).getTime() + HOURS * 3600e3).toISOString();
    api.GET("/passes", { params: { query: { satellite: satId, from, to } } }).then((r) => setPasses(r.data ?? []));
  }, [satId, from, api]);

  const t0 = from ? new Date(from).getTime() : 0;
  const tx = (iso: string) => LEFT + ((new Date(iso).getTime() - t0) / (HOURS * 3600e3)) * (W - LEFT - 10);
  const booked = new Map((bookings ?? []).map((b) => [b.pass_id, b]));
  const rows = (stations ?? []).map((s) => s.name);
  const nowX = clock && from ? tx(clock.sim_time) : -1;

  async function book() {
    if (!selected) return;
    const r = await api.POST("/bookings", { body: { pass_id: selected.id } });
    if (r.data) setMsg({ ok: true, text: `Booked ${r.data.station_name} pass at ${fmt(r.data.aos)}` });
    else setMsg({ ok: false, text: (r.error as { error?: string })?.error ?? "booking failed" });
  }

  async function addSatellite() {
    const lines = tle.trim().split(/\r?\n/).map((l) => l.trimEnd());
    if (lines.length < 3) return setMsg({ ok: false, text: "Paste 3 lines: name, TLE line 1, TLE line 2" });
    const norad = parseInt(lines[1].slice(2, 7), 10);
    const r = await api.POST("/satellites", { body: { name: lines[0].trim(), norad_id: norad, tle1: lines[1], tle2: lines[2] } });
    if (r.data) {
      setMsg({ ok: true, text: `Added ${r.data.name}` });
      setTle("");
      setSatId(r.data.id);
    } else setMsg({ ok: false, text: (r.error as { error?: string })?.error ?? "failed" });
  }

  return (
    <>
      <h2>Pass timeline</h2>
      <div className="toolbar">
        <label>
          Satellite{" "}
          <select data-testid="satellite-select" value={satId ?? ""} onChange={(e) => setSatId(e.target.value)}>
            {(sats ?? []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </label>
        <button className="btn" onClick={() => clock && setFrom(clock.sim_time)}>Window from now</button>
        <span className="muted">Next 24 h of simulated time, {passes.length} passes</span>
      </div>

      <div className="panel">
        <svg viewBox={`0 0 ${W} ${rows.length * ROW + 30}`} className="gantt" data-testid="gantt">
          {Array.from({ length: HOURS / 3 + 1 }, (_, i) => {
            const px = LEFT + (i / (HOURS / 3)) * (W - LEFT - 10);
            const label = from ? new Date(t0 + i * 3 * 3600e3).toISOString().slice(11, 16) : "";
            return (
              <g key={i}>
                <line x1={px} x2={px} y1={14} y2={rows.length * ROW + 20} className="grid" />
                <text x={px} y={10} className="axis" textAnchor="middle">{label}</text>
              </g>
            );
          })}
          {rows.map((r, i) => (
            <g key={r}>
              <text x={8} y={30 + i * ROW + 14} className="rowlabel">{r}</text>
              <line x1={LEFT} x2={W - 10} y1={22 + (i + 1) * ROW - 4} y2={22 + (i + 1) * ROW - 4} className="grid" />
            </g>
          ))}
          {passes.map((p) => {
            const row = rows.indexOf(p.station_name);
            if (row < 0) return null;
            const b = booked.get(p.id);
            const x1 = tx(p.aos);
            const w = Math.max(5, tx(p.los) - x1);
            return (
              <rect
                key={p.id}
                data-testid={`pass-${p.station_name}`}
                x={x1}
                y={26 + row * ROW}
                width={w}
                height={26}
                rx={4}
                className={`bar ${b ? "booked" : ""} ${selected?.id === p.id ? "selected" : ""}`}
                onClick={() => setSelected(p)}
              >
                <title>{`${p.station_name} ${fmt(p.aos)} → ${fmt(p.los)} max el ${p.max_elevation_deg.toFixed(1)}°`}</title>
              </rect>
            );
          })}
          {nowX >= LEFT && <line x1={nowX} x2={nowX} y1={14} y2={rows.length * ROW + 24} className="now" />}
        </svg>
      </div>

      <div className="two">
        <div className="panel" data-testid="pass-detail">
          <h3>Selected pass</h3>
          {selected ? (
            <>
              <dl>
                <dt>Station</dt><dd>{selected.station_name}</dd>
                <dt>AOS</dt><dd>{fmt(selected.aos)}</dd>
                <dt>LOS</dt><dd>{fmt(selected.los)}</dd>
                <dt>Max elevation</dt><dd>{selected.max_elevation_deg.toFixed(1)}°</dd>
              </dl>
              <Sparkline pass={selected} />
              <button className="btn primary" data-testid="book" disabled={booked.has(selected.id)} onClick={book}>
                {booked.has(selected.id) ? "Already booked" : "Book this pass"}
              </button>
            </>
          ) : <p className="muted">Click a bar in the timeline to inspect and book a pass.</p>}
          {msg && <p data-testid="message" className={msg.ok ? "ok" : "err"}>{msg.text}</p>}
        </div>
        <div className="panel">
          <h3>Add satellite</h3>
          <textarea data-testid="tle-input" rows={4} placeholder={"NAME\n1 25544U ...\n2 25544 ..."} value={tle} onChange={(e) => setTle(e.target.value)} />
          <button className="btn" data-testid="add-satellite" onClick={addSatellite}>Add from TLE</button>
        </div>
      </div>

      <h3>Bookings</h3>
      <table data-testid="bookings-table">
        <thead><tr><th>Station</th><th>Satellite</th><th>AOS</th><th>LOS</th><th>Status</th><th>Lock</th><th>Mission data</th></tr></thead>
        <tbody>
          {(bookings ?? []).map((b) => (
            <tr key={b.id}>
              <td>{b.station_name}</td><td>{b.satellite_name}</td><td>{fmt(b.aos)}</td><td>{fmt(b.los)}</td>
              <td><span className={`chip ${b.status}`} data-testid="booking-status">{b.status}</span></td>
              <td>{b.lock_ratio ? `${Math.round(b.lock_ratio * 100)}%` : "–"}</td>
              <td>{b.bytes_delivered ? `${(b.bytes_delivered / 1024).toFixed(1)} KiB` : "–"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

function Sparkline({ pass }: { pass: Pass }) {
  const pts = pass.track;
  const w = 260, h = 60;
  const t0 = new Date(pts[0].t).getTime(), span = new Date(pts[pts.length - 1].t).getTime() - t0 || 1;
  const d = pts.map((p) => `${((new Date(p.t).getTime() - t0) / span) * w},${h - (p.el / 90) * h}`).join(" ");
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="spark">
      <polyline points={d} fill="none" strokeWidth={2} className="line el" />
    </svg>
  );
}
