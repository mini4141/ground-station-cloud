import { useApi } from "../App";
import type { LiveState, Station } from "../api/client";
import { usePoll } from "../usePoll";

// Rough continent outlines (lon, lat) for a dependency-free equirectangular map.
const LAND: [number, number][][] = [
  [[-168, 66], [-156, 71], [-125, 70], [-95, 72], [-80, 73], [-62, 60], [-55, 50], [-67, 44], [-76, 35], [-81, 25], [-97, 26], [-105, 22], [-117, 32], [-125, 42], [-124, 48], [-135, 58], [-150, 60]],
  [[-105, 22], [-97, 16], [-86, 12], [-78, 8], [-72, 12], [-62, 10], [-50, 0], [-35, -6], [-40, -22], [-48, -28], [-58, -38], [-65, -42], [-68, -52], [-74, -50], [-72, -35], [-71, -18], [-80, -5], [-78, 2], [-85, 10], [-93, 16]],
  [[-10, 36], [-9, 43], [-2, 48], [5, 52], [10, 56], [22, 60], [30, 70], [60, 70], [100, 78], [140, 72], [170, 68], [180, 65], [160, 58], [142, 52], [135, 42], [122, 38], [122, 30], [110, 20], [105, 10], [100, 2], [95, 15], [88, 22], [78, 8], [72, 20], [58, 25], [50, 30], [36, 36], [28, 40], [20, 38], [12, 44], [3, 42]],
  [[-17, 21], [-10, 33], [10, 37], [32, 31], [43, 12], [51, 12], [40, -15], [35, -25], [20, -35], [12, -18], [9, 4], [-8, 5], [-17, 14]],
  [[114, -22], [122, -18], [136, -12], [142, -11], [153, -26], [150, -37], [140, -38], [130, -32], [115, -34]],
  [[-55, 60], [-45, 60], [-20, 70], [-20, 82], [-60, 82], [-70, 76]],
  [[-180, -72], [180, -72], [180, -90], [-180, -90]],
];

const W = 720;
const H = 360;
const x = (lon: number) => ((lon + 180) / 360) * W;
const y = (lat: number) => ((90 - lat) / 180) * H;

export function StationsPage() {
  const api = useApi();
  const stations = usePoll<Station[]>(async () => (await api.GET("/stations")).data ?? [], 3000);
  const live = usePoll<Record<string, LiveState>>(async () => {
    const out: Record<string, LiveState> = {};
    for (const s of stations ?? []) {
      const r = await api.GET("/stations/{id}/live", { params: { path: { id: s.id } } });
      if (r.data) out[s.name] = r.data;
    }
    return out;
  }, 2000, [stations?.length]);

  return (
    <>
      <h2>Ground station network</h2>
      <p className="muted">Stations are managed as code through the custom Terraform provider.</p>
      <div className="panel">
        <svg viewBox={`0 0 ${W} ${H}`} className="map" data-testid="stations-map">
          {[-60, -30, 0, 30, 60].map((lat) => <line key={lat} x1={0} x2={W} y1={y(lat)} y2={y(lat)} className="grid" />)}
          {[-120, -60, 0, 60, 120].map((lon) => <line key={lon} y1={0} y2={H} x1={x(lon)} x2={x(lon)} className="grid" />)}
          {LAND.map((poly, i) => <polygon key={i} className="land" points={poly.map(([lo, la]) => `${x(lo)},${y(la)}`).join(" ")} />)}
          {(stations ?? []).map((s) => {
            const state = live?.[s.name]?.state ?? "offline";
            return (
              <g key={s.id} transform={`translate(${x(s.longitude)},${y(s.latitude)})`}>
                <circle r={state === "tracking" ? 9 : 6} className={`pin ${state}`} />
                <text x={10} y={4} className="pin-label">{s.name}</text>
              </g>
            );
          })}
        </svg>
      </div>
      <table data-testid="stations-table">
        <thead>
          <tr><th>Station</th><th>Lat</th><th>Lon</th><th>Alt (m)</th><th>Band</th><th>Rate / min</th><th>Agent</th></tr>
        </thead>
        <tbody>
          {(stations ?? []).map((s) => {
            const st = live?.[s.name];
            return (
              <tr key={s.id}>
                <td><b>{s.name}</b></td>
                <td>{s.latitude.toFixed(2)}</td>
                <td>{s.longitude.toFixed(2)}</td>
                <td>{s.altitude_m ?? 0}</td>
                <td>{s.band}</td>
                <td>{(s.rate_per_min_minor / 100).toFixed(2)}</td>
                <td><span className={`chip ${st?.state ?? "offline"}`}>{st?.state ?? "offline"}</span></td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </>
  );
}
