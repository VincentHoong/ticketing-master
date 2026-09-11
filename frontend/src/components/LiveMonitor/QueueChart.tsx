import { useMemo } from "react";
import type { Snapshot } from "../../types/api";
import styles from "./QueueChart.module.css";

interface QueueChartProps {
  history: Snapshot[];
  maxConcurrent: number;
}

const WIDTH = 600;
const QUEUE_H = 120;
const GATE_H = 58;
const GAP = 14;
const HEIGHT = QUEUE_H + GAP + GATE_H;
const PAD_L = 44;
const PAD_T = 10;

function niceCeil(v: number): number {
  if (v <= 5) return 5;
  const mag = Math.pow(10, Math.floor(Math.log10(v)));
  return Math.ceil(v / mag) * mag;
}

export function QueueChart({ history, maxConcurrent }: QueueChartProps) {
  const chart = useMemo(() => {
    if (history.length === 0) return null;

    const innerW = WIDTH - PAD_L - 4;
    const n = history.length;

    // x is elapsed time, not array index, so a paused or recovered run does not
    // silently rescale the axis.
    const t0 = history[0].elapsedMs;
    const tEnd = history[n - 1].elapsedMs;
    const span = Math.max(tEnd - t0, 1);
    const x = (s: Snapshot) => PAD_L + (n === 1 ? innerW : ((s.elapsedMs - t0) / span) * innerW);

    const queueMax = niceCeil(Math.max(...history.map((s) => s.inQueue), 1));
    // The gate lane is scaled to the cap itself, so a whitelist pinned at the cap
    // reads as a flat line at the top rather than a flat line on zero.
    const gateMax = niceCeil(Math.max(maxConcurrent, ...history.map((s) => s.whitelistSize), 1));

    const yQueue = (v: number) => PAD_T + QUEUE_H - (v / queueMax) * QUEUE_H;
    const gateTop = PAD_T + QUEUE_H + GAP;
    const yGate = (v: number) => gateTop + GATE_H - (v / gateMax) * GATE_H;

    const toPath = (pts: readonly (readonly [number, number])[]) =>
      pts.map(([px, py], i) => `${i === 0 ? "M" : "L"}${px.toFixed(1)},${py.toFixed(1)}`).join(" ");

    const queuePts = history.map((s) => [x(s), yQueue(s.inQueue)] as const);
    const gatePts = history.map((s) => [x(s), yGate(s.whitelistSize)] as const);

    const queuePath = toPath(queuePts);
    const baseline = PAD_T + QUEUE_H;
    const queueArea =
      n === 1
        ? ""
        : `${queuePath} L${x(history[n - 1]).toFixed(1)},${baseline} L${x(history[0]).toFixed(1)},${baseline} Z`;

    return {
      queuePath,
      queueArea,
      gatePath: toPath(gatePts),
      queueMax,
      gateMax,
      gateTop,
      capY: yGate(maxConcurrent),
      elapsedS: (tEnd - t0) / 1000,
      single: n === 1,
    };
  }, [history, maxConcurrent]);

  if (!chart) {
    return <div className={styles.empty}>No data yet — run a simulation to see the gate hold the line.</div>;
  }

  return (
    <div className={styles.wrap}>
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className={styles.svg}
        preserveAspectRatio="xMidYMid meet"
        role="img"
        aria-label={`Queue depth falling to zero while admitted users stay capped at ${maxConcurrent}`}
      >
        {[0, 0.5, 1].map((f) => {
          const y = PAD_T + QUEUE_H * (1 - f);
          return <line key={f} x1={PAD_L} x2={WIDTH - 4} y1={y} y2={y} stroke="var(--ink)" strokeOpacity={0.12} />;
        })}
        <text x={2} y={PAD_T + 8} className={styles.axisLabel}>
          {chart.queueMax}
        </text>
        <text x={2} y={PAD_T + QUEUE_H} className={styles.axisLabel}>
          0
        </text>
        {chart.queueArea && <path d={chart.queueArea} fill="var(--blue)" fillOpacity={0.18} />}
        <path d={chart.queuePath} fill="none" stroke="var(--blue)" strokeWidth={2.5} />

        <line
          x1={PAD_L}
          x2={WIDTH - 4}
          y1={chart.capY}
          y2={chart.capY}
          stroke="var(--ink)"
          strokeWidth={1.5}
          strokeDasharray="4 4"
          strokeOpacity={0.55}
        />
        <line
          x1={PAD_L}
          x2={WIDTH - 4}
          y1={chart.gateTop + GATE_H}
          y2={chart.gateTop + GATE_H}
          stroke="var(--ink)"
          strokeOpacity={0.12}
        />
        <text x={2} y={chart.capY + 4} className={styles.axisLabel}>
          cap {maxConcurrent}
        </text>
        <path d={chart.gatePath} fill="none" stroke="var(--ink)" strokeWidth={2.5} />

        {chart.single && (
          <text x={PAD_L + 8} y={PAD_T + QUEUE_H / 2} className={styles.axisLabel}>
            waiting for more samples…
          </text>
        )}
      </svg>
      <div className={styles.legend}>
        <span className={styles.legendItem}>
          <span className={styles.swatch} style={{ background: "var(--blue)" }} /> in queue (top)
        </span>
        <span className={styles.legendItem}>
          <span className={styles.swatch} style={{ background: "var(--ink)" }} /> admitted (bottom, own scale)
        </span>
        <span className={styles.legendItem}>{chart.elapsedS.toFixed(1)}s of run</span>
      </div>
    </div>
  );
}
