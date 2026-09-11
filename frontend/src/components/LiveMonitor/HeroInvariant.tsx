import type { Snapshot } from "../../types/api";
import styles from "./HeroInvariant.module.css";

interface HeroInvariantProps {
  snapshot: Snapshot | null;
}

const GUARANTEE = "guaranteed by SELECT … FOR UPDATE on the event row — the queue only bounds latency";

export function HeroInvariant({ snapshot }: HeroInvariantProps) {
  if (!snapshot) {
    return (
      <div className={`${styles.hero} ${styles.idle}`}>
        <div className={styles.label}>seats sold / capacity</div>
        <div className={styles.value}>— / —</div>
        <div className={styles.status}>NO RUN YET</div>
        <div className={styles.footnote}>{GUARANTEE}</div>
      </div>
    );
  }

  const { seatsSold, capacity, capacityKnown, oversold, done } = snapshot;
  const unknown = !capacityKnown;

  // Only claim the invariant once the run is over. Announcing "zero overselling holds"
  // while 0 of 100 seats are sold is an unearned verdict.
  const verdict = unknown
    ? { cls: styles.unknown, text: "CAPACITY UNKNOWN — CANNOT CONFIRM" }
    : oversold
      ? { cls: styles.oversold, text: "⚠ OVERSOLD — INVARIANT VIOLATED" }
      : done
        ? { cls: styles.safe, text: "ZERO OVERSELLING HOLDS" }
        : { cls: styles.pending, text: "SELLING — INVARIANT HOLDING SO FAR" };

  return (
    <div className={`${styles.hero} ${verdict.cls}`} aria-live="polite">
      <div className={styles.label}>seats sold / capacity</div>
      <div className={styles.value}>
        {seatsSold} <span className={styles.slash}>/</span> {unknown ? "UNKNOWN" : capacity}
      </div>
      <div className={styles.status}>{verdict.text}</div>
      <div className={styles.footnote}>{GUARANTEE}</div>
    </div>
  );
}
