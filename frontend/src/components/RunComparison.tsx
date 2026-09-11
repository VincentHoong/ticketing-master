import { useMemo } from "react";
import type { CompletedRun } from "../hooks/useSimulationRun";
import styles from "./RunComparison.module.css";

function peakPg(run: CompletedRun): number {
  return run.pgPeak;
}

export function RunComparison({ runs }: { runs: CompletedRun[] }) {
  // The argument is the contrast between the tightest and loosest gate, not a list
  // of runs. Pull those two out and state the delta rather than making the viewer
  // divide two columns in their head.
  const verdict = useMemo(() => {
    const withCapacity = runs.filter((r) => r.snapshot.capacityKnown);
    if (withCapacity.length < 2) return null;

    // Compare within a single transport. An in-process run against an HTTP run differs by
    // the entire request path, and charging that gap to the gate would make the headline
    // claim simply wrong.
    const transport = withCapacity[withCapacity.length - 1].snapshot.transport;
    const usable = withCapacity.filter((r) => r.snapshot.transport === transport);
    if (usable.length < 2) return null;

    const sorted = [...usable].sort((a, b) => a.snapshot.maxConcurrent - b.snapshot.maxConcurrent);
    const tight = sorted[0];
    const loose = sorted[sorted.length - 1];
    if (tight.snapshot.maxConcurrent === loose.snapshot.maxConcurrent) return null;

    const ratio = tight.snapshot.reserveP95Ms > 0 ? loose.snapshot.reserveP95Ms / tight.snapshot.reserveP95Ms : 0;
    const neverOversold = usable.every((r) => !r.snapshot.oversold);

    return { tight, loose, ratio, neverOversold, count: usable.length };
  }, [runs]);

  return (
    <section className={`panel ${styles.panel}`}>
      <span className="panel-label">05 / run comparison</span>

      {runs.length === 0 && (
        <p className={styles.empty}>
          Finished runs appear here. Run <strong>Tight (10)</strong> then <strong>No gate (3000)</strong> against the
          same event — on the same transport — to see the trade-off.
        </p>
      )}

      {verdict && (
        <div className={styles.verdict}>
          <div className={styles.verdictHead}>
            cap {verdict.tight.snapshot.maxConcurrent} vs cap {verdict.loose.snapshot.maxConcurrent}
          </div>
          <div className={styles.verdictGrid}>
            <Delta
              label="reserve p95"
              from={`${verdict.tight.snapshot.reserveP95Ms.toFixed(1)}ms`}
              to={`${verdict.loose.snapshot.reserveP95Ms.toFixed(1)}ms`}
              note={verdict.ratio >= 1.1 ? `${verdict.ratio.toFixed(1)}× worse without the gate` : "comparable"}
            />
            <Delta
              label="postgres pool peak"
              from={`${peakPg(verdict.tight)} / ${verdict.tight.snapshot.pgMax}`}
              to={`${peakPg(verdict.loose)} / ${verdict.loose.snapshot.pgMax}`}
              note="connections held"
            />
            <Delta
              label="wall clock"
              from={`${(verdict.tight.snapshot.elapsedMs / 1000).toFixed(1)}s`}
              to={`${(verdict.loose.snapshot.elapsedMs / 1000).toFixed(1)}s`}
              note="the gate costs total drain time"
            />
          </div>
          <div className={verdict.neverOversold ? styles.verdictSafe : styles.verdictBad}>
            {verdict.neverOversold
              ? `zero overselling across all ${verdict.count} runs — the row lock holds regardless of the gate`
              : "a run oversold — the invariant was violated"}
          </div>
        </div>
      )}

      {runs.length > 0 && (
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>run</th>
                <th>transport</th>
                <th>users</th>
                <th>max concurrent</th>
                <th>seats sold / cap</th>
                <th>oversold</th>
                <th>reserve p95</th>
                <th>wall clock</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => (
                <tr
                  key={r.runId}
                  className={
                    r.snapshot.oversold ? styles.badRow : !r.snapshot.capacityKnown ? styles.unknownRow : undefined
                  }
                >
                  <td className={styles.runId}>{r.runId.slice(0, 8)}</td>
                  <td>{r.snapshot.transport === "http" ? "http" : "in-proc"}</td>
                  <td>{r.snapshot.totalUsers}</td>
                  <td>{r.snapshot.maxConcurrent}</td>
                  <td>
                    {r.snapshot.seatsSold} / {r.snapshot.capacityKnown ? r.snapshot.capacity : "?"}
                  </td>
                  <td>{!r.snapshot.capacityKnown ? "UNKNOWN" : r.snapshot.oversold ? "YES" : "no"}</td>
                  <td>{r.snapshot.reserveP95Ms.toFixed(1)}ms</td>
                  <td>{(r.snapshot.elapsedMs / 1000).toFixed(1)}s</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Delta({ label, from, to, note }: { label: string; from: string; to: string; note: string }) {
  return (
    <div className={styles.delta}>
      <div className={styles.deltaLabel}>{label}</div>
      <div className={styles.deltaValues}>
        <span className={styles.deltaFrom}>{from}</span>
        <span className={styles.deltaArrow}>→</span>
        <span className={styles.deltaTo}>{to}</span>
      </div>
      <div className={styles.deltaNote}>{note}</div>
    </div>
  );
}
