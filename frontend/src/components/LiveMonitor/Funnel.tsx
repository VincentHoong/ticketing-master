import type { Snapshot } from "../../types/api";
import styles from "./Funnel.module.css";

interface FunnelProps {
  snapshot: Snapshot;
}

interface Bar {
  label: string;
  value: number;
  color: string;
  scale?: number;
  note?: string;
}

export function Funnel({ snapshot }: FunnelProps) {
  const { totalUsers, enqueued, admitted, reserved, rejectedCapacity, rejectedQuota, timedOut, cancelled, reaped, errors } =
    snapshot;

  // reserved is scaled against capacity, not the user count: 100 of 5000 renders as a
  // 2% sliver, which hides the one bar the whole demo is about.
  const capacityScale = snapshot.capacityKnown && snapshot.capacity > 0 ? snapshot.capacity : undefined;
  const funnel: Bar[] = [
    { label: "enqueued", value: enqueued, color: "var(--ink)" },
    { label: "admitted", value: admitted, color: "var(--blue)" },
    {
      label: "reserved",
      value: reserved,
      color: "var(--lime)",
      scale: capacityScale,
      note: capacityScale ? `of ${capacityScale} seats` : undefined,
    },
  ];

  const terminal: Bar[] = [
    { label: "rejected: capacity", value: rejectedCapacity, color: "var(--ink)" },
    { label: "rejected: quota", value: rejectedQuota, color: "var(--ink)" },
    { label: "timed out", value: timedOut, color: "var(--ink)" },
    { label: "cancelled", value: cancelled, color: "var(--ink)" },
    // Reaped users left the queue without an outcome. Omitting them made the accounting
    // line short by exactly their count, with nothing on screen to explain the shortfall.
    { label: "reaped", value: reaped, color: reaped > 0 ? "var(--pink)" : "var(--ink)" },
    { label: "errors", value: errors, color: "var(--pink)" },
  ];

  const accountedFor = reserved + rejectedCapacity + rejectedQuota + timedOut + cancelled + reaped + errors;
  const max = Math.max(totalUsers, 1);

  return (
    <div className={styles.wrap}>
      <div className={styles.section}>
        {funnel.map((bar) => (
          <FunnelBar key={bar.label} bar={bar} max={max} />
        ))}
      </div>
      <div className={`${styles.section} ${styles.terminalSection}`}>
        {terminal.map((bar) => (
          <FunnelBar key={bar.label} bar={bar} max={max} small />
        ))}
      </div>
      <div className={styles.accounted}>
        accounted <span className="mono">{accountedFor}</span> / <span className="mono">{totalUsers}</span>
        {" · "}in queue <span className="mono">{snapshot.inQueue}</span>
        {" · "}whitelist <span className="mono">{snapshot.whitelistSize}</span>
      </div>
    </div>
  );
}

function FunnelBar({ bar, max, small }: { bar: Bar; max: number; small?: boolean }) {
  const denominator = bar.scale ?? max;
  const pct = Math.min(100, (bar.value / Math.max(denominator, 1)) * 100);
  return (
    <div className={`${styles.row} ${small ? styles.small : ""}`}>
      <div className={styles.rowLabel}>
        {bar.label}
        {bar.note && <span className={styles.note}> {bar.note}</span>}
      </div>
      <div className={styles.track}>
        <div className={styles.fill} style={{ width: `${pct}%`, background: bar.color }} />
      </div>
      <div className={styles.rowValue}>{bar.value}</div>
    </div>
  );
}
