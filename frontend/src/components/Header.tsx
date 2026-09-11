import { API_BASE } from "../api/client";
import type { HealthState } from "../hooks/useHealth";
import styles from "./Header.module.css";

export function Header({ health, reachable, checking }: HealthState) {
  const pgOk = health?.postgres === true;
  const redisOk = health?.redis === true;

  return (
    <header className={styles.header}>
      <div className={styles.top}>
        <div className={styles.brand}>
          <span className={styles.brandMark}>▣</span>
          <div>
            <h1 className={styles.title}>Ticketing Master</h1>
            <p className={styles.subtitle}>concurrency proof / live load test</p>
          </div>
        </div>

        <div className={styles.healthRow}>
          <HealthDot label="API" ok={reachable} checking={checking} />
          <HealthDot label="PG" ok={pgOk} checking={checking || !reachable} />
          <HealthDot label="REDIS" ok={redisOk} checking={checking || !reachable} />
        </div>
      </div>

      {!reachable && !checking && (
        <div className={styles.banner}>
          <strong>BACKEND UNREACHABLE</strong> — cannot reach {API_BASE}. Start the backend
          with <code>DEMO_MODE=true</code> and confirm it is listening on that address.
        </div>
      )}
      {reachable && !checking && health && (!pgOk || !redisOk) && (
        <div className={styles.banner}>
          <strong>DEGRADED</strong> — {!pgOk && "postgres down. "}{!redisOk && "redis down. "}
          The simulation will not behave correctly until both are healthy.
        </div>
      )}
    </header>
  );
}

function HealthDot({ label, ok, checking }: { label: string; ok: boolean; checking: boolean }) {
  const state = checking ? "checking" : ok ? "ok" : "bad";
  const glyph = checking ? "…" : ok ? "✓" : "✕";
  const spoken = checking ? "checking" : ok ? "healthy" : "unreachable";
  return (
    <div className={styles.healthItem} title={`${label}: ${spoken}`}>
      <span className={`${styles.dot} ${styles[state]}`} aria-hidden="true" />
      <span className={styles.healthLabel}>
        <span aria-hidden="true">{glyph} </span>
        {label}
      </span>
      <span className="sr-only">{`${label}: ${spoken}`}</span>
    </div>
  );
}
