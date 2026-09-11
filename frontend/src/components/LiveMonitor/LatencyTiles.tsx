import type { Snapshot } from "../../types/api";
import styles from "./LatencyTiles.module.css";

export function LatencyTiles({ snapshot }: { snapshot: Snapshot }) {
  const fmt = (v: number) => `${v.toFixed(2)}ms`;
  // Seconds of queue wait are the expected outcome of admission control, not a fault.
  // Rendering them at the same weight as reserve latency reads as "something is broken".
  const wait = (v: number) => (v >= 1000 ? `${(v / 1000).toFixed(1)}s` : `${v.toFixed(0)}ms`);
  return (
    <div className={styles.wrap}>
      <div className={styles.group}>
        <div className={styles.groupLabel}>reserve latency <span className={styles.hint}>database contention</span></div>
        <div className={styles.tiles}>
          <Tile label="p50" value={fmt(snapshot.reserveP50Ms)} />
          <Tile label="p95" value={fmt(snapshot.reserveP95Ms)} />
          <Tile label="p99" value={fmt(snapshot.reserveP99Ms)} />
        </div>
      </div>
      <div className={styles.group}>
        <div className={styles.groupLabel}>
          time spent waiting in queue{" "}
          <span className={styles.hint}>
            expected — this is the gate working. Quantised by pollIntervalMs={snapshot.pollIntervalMs}ms, so not true
            admission latency.
          </span>
        </div>
        <div className={styles.secondaryRow}>
          <span className={styles.secondaryItem}>
            p50 <strong>{wait(snapshot.queueWaitP50Ms)}</strong>
          </span>
          <span className={styles.secondaryItem}>
            p95 <strong>{wait(snapshot.queueWaitP95Ms)}</strong>
          </span>
        </div>
      </div>
    </div>
  );
}

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="stat-tile">
      <div className="stat-label">{label}</div>
      <div className="stat-value">{value}</div>
    </div>
  );
}
