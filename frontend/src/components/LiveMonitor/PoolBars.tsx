import type { Snapshot } from "../../types/api";
import styles from "./PoolBars.module.css";

export function PoolBars({ snapshot, history }: { snapshot: Snapshot; history: Snapshot[] }) {
  // Instantaneous usage is ~0 the moment a run finishes, which is exactly when someone
  // points at this panel. Peak-so-far is what actually shows whether the gate spared
  // the database.
  const pgPeak = history.reduce((m, s) => Math.max(m, s.pgAcquired), snapshot.pgAcquired);
  const redisPeak = history.reduce(
    (m, s) => Math.max(m, s.redisTotalConns - s.redisIdleConns),
    snapshot.redisTotalConns - snapshot.redisIdleConns
  );
  const pgUsedPct = snapshot.pgMax > 0 ? (pgPeak / snapshot.pgMax) * 100 : 0;
  const redisUsedPct = snapshot.redisTotalConns > 0 ? (redisPeak / snapshot.redisTotalConns) * 100 : 0;
  const pgSaturated = snapshot.pgMax > 0 && pgPeak >= snapshot.pgMax - 1;

  return (
    <div className={styles.wrap}>
      <PoolBar
        label="postgres pool — peak"
        used={pgPeak}
        total={snapshot.pgMax}
        pct={pgUsedPct}
        extra={pgSaturated ? "saturated — the gate did not spare the database" : `now ${snapshot.pgAcquired} · idle ${snapshot.pgIdle}`}
        warn={pgSaturated}
      />
      <PoolBar
        label="redis pool — peak"
        used={redisPeak}
        total={snapshot.redisTotalConns}
        pct={redisUsedPct}
        extra={`now ${snapshot.redisTotalConns - snapshot.redisIdleConns} · timeouts ${snapshot.redisTimeouts}`}
        warn={snapshot.redisTimeouts > 0}
      />
    </div>
  );
}

function PoolBar({
  label,
  used,
  total,
  pct,
  extra,
  warn,
}: {
  label: string;
  used: number;
  total: number;
  pct: number;
  extra: string;
  warn?: boolean;
}) {
  return (
    <div className={styles.bar}>
      <div className={styles.barHead}>
        <span className={styles.barLabel}>{label}</span>
        <span className={styles.barValue}>
          {used} / {total}
        </span>
      </div>
      <div className={styles.track}>
        <div
          className={styles.fill}
          style={{ width: `${Math.min(100, pct)}%`, background: warn ? "var(--pink)" : "var(--blue)" }}
        />
      </div>
      <div className={styles.extra}>{extra}</div>
    </div>
  );
}
