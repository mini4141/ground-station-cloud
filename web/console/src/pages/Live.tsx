import { useEffect, useRef, useState } from "react";
import { useApi } from "../App";
import type { LiveState, Station } from "../api/client";
import { usePoll } from "../usePoll";

type Frame = NonNullable<LiveState["frame"]>;

export function LivePage() {
  const api = useApi();
  const stations = usePoll<Station[]>(async () => (await api.GET("/stations")).data ?? [], 5000);
  const [stationId, setStationId] = useState<string>();
  const [frames, setFrames] = useState<Frame[]>([]);
  const lastT = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!stationId && stations?.length) setStationId(stations[0].id);
  }, [stations, stationId]);
  useEffect(() => {
    setFrames([]);
    lastT.current = undefined;
  }, [stationId]);

  const live = usePoll<LiveState | undefined>(
    async () => (stationId ? (await api.GET("/stations/{id}/live", { params: { path: { id: stationId } } })).data : undefined),
    1000,
    [stationId],
  );
  useEffect(() => {
    const f = live?.frame;
    if (f && f.t !== lastT.current) {
      lastT.current = f.t;
      setFrames((prev) => [...prev.slice(-119), f]);
    }
  }, [live]);

  const f = live?.frame;
  const state = live?.state ?? "offline";
  return (
    <>
      <h2>Live telemetry</h2>
      <div className="toolbar">
        {(stations ?? []).map((s) => (
          <button key={s.id} data-testid={`station-${s.name}`} className={`btn ${s.id === stationId ? "primary" : ""}`} onClick={() => setStationId(s.id)}>{s.name}</button>
        ))}
        <span className={`chip ${state}`} data-testid="station-state">{state}</span>
        <span className="muted">polling /stations/{"{id}"}/live every second · {frames.length} frames</span>
      </div>
      <div className="tiles">
        <Tile label="Lock" value={f ? (f.lock ? "LOCKED" : "no lock") : "–"} cls={f?.lock ? "good" : ""} testid="tile-lock" />
        <Tile label="SNR" value={f ? `${f.snr.toFixed(1)} dB` : "–"} testid="tile-snr" />
        <Tile label="Azimuth" value={f ? `${f.az.toFixed(0)}°` : "–"} />
        <Tile label="Elevation" value={f ? `${f.el.toFixed(0)}°` : "–"} />
        <Tile label="Bitrate" value={f ? `${f.bitrate} bps` : "–"} />
      </div>
      <div className="two">
        <Chart title="SNR (dB)" data={frames.map((x) => x.snr)} max={25} cls="snr" testid="chart-snr" />
        <Chart title="Elevation (°)" data={frames.map((x) => x.el)} max={90} cls="el" testid="chart-el" />
      </div>
    </>
  );
}

function Tile({ label, value, cls = "", testid }: { label: string; value: string; cls?: string; testid?: string }) {
  return (
    <div className={`tile ${cls}`} data-testid={testid}>
      <div className="tile-label">{label}</div>
      <div className="tile-value">{value}</div>
    </div>
  );
}

function Chart({ title, data, max, cls, testid }: { title: string; data: number[]; max: number; cls: string; testid: string }) {
  const w = 480, h = 180, pad = 24;
  const pts = data.map((v, i) => `${pad + (i / Math.max(data.length - 1, 29)) * (w - pad - 6)},${h - pad - (Math.min(v, max) / max) * (h - pad - 8)}`).join(" ");
  return (
    <div className="panel">
      <h3>{title}</h3>
      <svg viewBox={`0 0 ${w} ${h}`} className="chart" data-testid={testid} data-points={data.length}>
        {[0, 0.5, 1].map((g) => (
          <g key={g}>
            <line x1={pad} x2={w - 6} y1={h - pad - g * (h - pad - 8)} y2={h - pad - g * (h - pad - 8)} className="grid" />
            <text x={2} y={h - pad - g * (h - pad - 8) + 4} className="axis">{Math.round(g * max)}</text>
          </g>
        ))}
        {data.length > 1 && <polyline points={pts} fill="none" strokeWidth={2.5} className={`line ${cls}`} />}
      </svg>
    </div>
  );
}
