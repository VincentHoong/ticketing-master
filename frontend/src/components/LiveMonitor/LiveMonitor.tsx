import type { Snapshot } from "../../types/api";
import { HeroInvariant } from "./HeroInvariant";
import { Funnel } from "./Funnel";
import { QueueChart } from "./QueueChart";
import { LatencyTiles } from "./LatencyTiles";
import { PoolBars } from "./PoolBars";
import { ErrorList } from "./ErrorList";
import styles from "./LiveMonitor.module.css";

interface LiveMonitorProps {
  snapshot: Snapshot | null;
  history: Snapshot[];
  running: boolean;
  reconnecting: boolean;
}

export function LiveMonitor({ snapshot, history, running, reconnecting }: LiveMonitorProps) {
  // Completed work, not totalUsers: dividing a constant by elapsed time renders an
  // absurd number on the first tick and then decays for the rest of the run.
  const settled = snapshot
    ? snapshot.reserved +
      snapshot.rejectedCapacity +
      snapshot.rejectedQuota +
      snapshot.timedOut +
      snapshot.cancelled +
      snapshot.reaped +
      snapshot.errors
    : 0;
  const throughput = snapshot && snapshot.elapsedMs > 0 ? (settled / snapshot.elapsedMs) * 1000 : 0;

  return (
    <section className={styles.wrap}>
      <div className={styles.headRow}>
        <span className="panel-label">04 / live monitor</span>
        {snapshot && (
          <div className={styles.meta}>
            <span className="tag">{reconnecting ? "RECONNECTING" : running ? "RUNNING" : "DONE"}</span>
            <span
              className={styles.elapsed}
              title={
                snapshot.transport === "http"
                  ? "Full request path: socket, middleware, JWT, JSON"
                  : "Service layer called directly: no socket, auth or JSON"
              }
            >
              {snapshot.transport === "http" ? "over HTTP" : "in-process"}
            </span>
            <span className={styles.elapsed}>elapsed {(snapshot.elapsedMs / 1000).toFixed(1)}s</span>
            <span className={styles.elapsed} title="settled users per second">
              {throughput.toFixed(0)} users/s settled
            </span>
          </div>
        )}
      </div>

      <HeroInvariant snapshot={snapshot} />

      {snapshot && (
        <div className={styles.grid}>
          <div className={`panel ${styles.cell}`}>
            <div className={styles.cellLabel}>funnel</div>
            <Funnel snapshot={snapshot} />
          </div>

          <div className={`panel ${styles.cell}`}>
            <div className={styles.cellLabel}>queue vs admitted (the gate holding the line)</div>
            <QueueChart history={history} maxConcurrent={snapshot.maxConcurrent} />
          </div>

          <div className={`panel ${styles.cell}`}>
            <div className={styles.cellLabel}>latency</div>
            <LatencyTiles snapshot={snapshot} />
          </div>

          <div className={`panel ${styles.cell}`}>
            <div className={styles.cellLabel}>connection pools</div>
            <PoolBars snapshot={snapshot} history={history} />
          </div>

          {Object.keys(snapshot.errorCounts).length > 0 && (
            <div className={styles.cellWide}>
              <ErrorList errorCounts={snapshot.errorCounts} />
            </div>
          )}
        </div>
      )}

      {!snapshot && <p className={styles.empty}>Run a simulation to see it live here.</p>}
    </section>
  );
}
