import { useEffect, useState } from "react";
import type { EventItem, SimulateRequest, Transport } from "../types/api";
import styles from "./SimulationControls.module.css";

interface SimulationControlsProps {
  selectedEvent: EventItem | null;
  running: boolean;
  onRun: (req: SimulateRequest) => Promise<void> | void;
  onCancel: () => void;
  error: string | null;
}

const PRESETS = [
  { cap: 10, label: "Tight (10)", cls: "btn-lime", title: "Lowest latency, longest run: 3000 users take 14.2s at a 2.6ms reserve p50" },
  { cap: 50, label: "Balanced (50)", cls: "btn-blue", title: "The knee: 4.8s at a 25ms p50 — almost all the speed for a fraction of the latency" },
  { cap: 3000, label: "No gate (3000)", cls: "btn-pink", title: "Every user at once: 3.2s but a 1282ms p50 — 4.4x faster for 500x the latency" },
];

const TRANSPORTS: { id: Transport; label: string; title: string; maxUsers: number }[] = [
  {
    id: "inproc",
    label: "IN-PROCESS",
    title: "Calls the service layer directly. No socket, auth or JSON, so it scales to 20,000 users — use it to prove the oversell invariant.",
    maxUsers: 20000,
  },
  {
    id: "http",
    label: "OVER HTTP",
    title: "Real requests through the router: socket, middleware, JWT, JSON. Bounded by sockets, but these are the latencies a user would feel.",
    maxUsers: 4000,
  },
];

// Mirrors the backend's poll-interval scaling in simulation/engine.go.
const POLL_OPS_BUDGET = 50000;
const MIN_POLL_MS = 50;
const MAX_POLL_MS = 1000;

function pollIntervalMs(users: number): number {
  return Math.min(MAX_POLL_MS, Math.max(MIN_POLL_MS, Math.floor((users * 1000) / POLL_OPS_BUDGET)));
}

// Measured ceilings, used as a floor on the estimate: once the gate is wide enough to stop
// binding, the row lock — not admission — sets the pace.
const RESERVES_PER_SEC = { inproc: 1000, http: 700 };
const ENQUEUES_PER_SEC = { inproc: 20000, http: 1500 };

// An earlier version of this estimate assumed every user waits their turn through the gate,
// which overstated a sell-out run by more than an order of magnitude — 20k users against
// 1000 seats predicted 13 minutes and measured 35s. Only the users who can actually get a
// seat queue for admission; once the event sells out the rest leave on their next poll.
function estimateSeconds(
  users: number,
  maxConcurrent: number,
  capacity: number | null,
  transport: Transport,
): number {
  if (users <= 0 || maxConcurrent <= 0) return 0;

  const pollMs = pollIntervalMs(users);
  const admissions = capacity && capacity > 0 ? Math.min(users, capacity) : users;
  const gated = (admissions * (pollMs / 1000)) / maxConcurrent;
  const lockBound = admissions / RESERVES_PER_SEC[transport];
  const startup = users / ENQUEUES_PER_SEC[transport];
  const noticeSoldOut = (pollMs / 1000) * 2;

  return Math.max(gated, lockBound) + startup + noticeSoldOut;
}

function formatDuration(seconds: number): string {
  if (seconds < 60) return `~${Math.max(1, Math.round(seconds))}s`;
  if (seconds < 3600) return `~${Math.round(seconds / 60)} min`;
  return `~${(seconds / 3600).toFixed(1)} hours`;
}

function clampNumber(raw: string, fallback: number, min: number, max?: number): number {
  const parsed = Number(raw);
  if (raw.trim() === "" || Number.isNaN(parsed)) return fallback;
  if (parsed < min) return min;
  if (max !== undefined && parsed > max) return max;
  return parsed;
}

export function SimulationControls({ selectedEvent, running, onRun, onCancel, error }: SimulationControlsProps) {
  const [users, setUsers] = useState(5000);
  const [capacity, setCapacity] = useState<number | null>(null);
  const [maxConcurrent, setMaxConcurrent] = useState(10);
  const [transport, setTransport] = useState<Transport>("inproc");
  const [submitting, setSubmitting] = useState(false);

  const maxUsers = TRANSPORTS.find((t) => t.id === transport)!.maxUsers;

  // The backend rejects an HTTP run above its socket ceiling, so clamp on switch rather
  // than letting the user discover the limit as a failed run.
  useEffect(() => {
    setUsers((current) => Math.min(current, maxUsers));
  }, [maxUsers]);

  // Default the override to whatever the chosen event already has, so the hero number
  // cannot silently disagree with the card the viewer just clicked.
  useEffect(() => {
    setCapacity(selectedEvent ? selectedEvent.capacity : null);
  }, [selectedEvent]);

  const estimate = estimateSeconds(users, maxConcurrent, capacity, transport);
  const tooSlow = estimate > 120;
  const busy = running || submitting;
  const canRun = !!selectedEvent && !busy;
  const overridden = !!selectedEvent && capacity !== null && capacity !== selectedEvent.capacity;

  const run = async () => {
    if (!selectedEvent || submitting) return;
    setSubmitting(true);
    try {
      await onRun({
        eventId: selectedEvent.id,
        users,
        capacity: capacity ?? selectedEvent.capacity,
        maxConcurrent,
        // Without this every user gets the 60s server default, so any run longer than a
        // minute reports mass timeouts that are really just the gate still working.
        userTimeoutMs: Math.round(Math.max(60, estimate * 2) * 1000),
        reset: true,
        transport,
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className={`panel ${styles.panel}`}>
      <span className="panel-label">03 / run simulation</span>

      {!selectedEvent && <p className={styles.hint}>Select an event above to configure a run.</p>}

      <div className={styles.transportBlock}>
        <span className={styles.gateLabel}>Transport</span>
        <div className={styles.transportRow}>
          {TRANSPORTS.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`btn ${transport === t.id ? "btn-blue" : ""}`}
              disabled={busy}
              title={t.title}
              aria-pressed={transport === t.id}
              onClick={() => setTransport(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <p className={styles.gateHint}>
          {transport === "http" ? (
            <>
              Every user makes real requests — socket, middleware, JWT, JSON — so these latencies are
              what a user would actually feel. Capped at {maxUsers.toLocaleString()} users because each
              one holds a socket.
            </>
          ) : (
            <>
              Calls the service layer directly, skipping the socket, auth and JSON. Latencies are the
              cost of the critical section rather than end-to-end, which is what lets it run to{" "}
              {maxUsers.toLocaleString()} users and prove the oversell invariant at scale.
            </>
          )}
        </p>
      </div>

      <div className={styles.controlsRow}>
        <div className="field">
          <label htmlFor="users">Users</label>
          <input
            id="users"
            type="number"
            min={1}
            max={maxUsers}
            value={users}
            disabled={busy}
            onChange={(e) => setUsers(clampNumber(e.target.value, users, 1, maxUsers))}
          />
        </div>
        <div className="field">
          <label htmlFor="capacity">
            Capacity <span className={styles.subtle}>— overrides the event</span>
          </label>
          <input
            id="capacity"
            type="number"
            min={1}
            value={capacity ?? ""}
            disabled={busy || !selectedEvent}
            onChange={(e) => setCapacity(clampNumber(e.target.value, capacity ?? 1, 1))}
          />
          {overridden && (
            <span className={styles.override}>
              event is {selectedEvent.capacity} — this run writes {capacity}
            </span>
          )}
        </div>
      </div>

      <div className={styles.gateBlock}>
        <label htmlFor="maxConcurrent" className={styles.gateLabel}>
          Max Concurrent (the gate) — this is the thesis dial
        </label>
        <div className={styles.gateRow}>
          <input
            id="maxConcurrent"
            type="number"
            min={1}
            className={styles.gateInput}
            value={maxConcurrent}
            disabled={busy}
            onChange={(e) => setMaxConcurrent(clampNumber(e.target.value, maxConcurrent, 1))}
          />
          {PRESETS.map((p) => (
            <button
              key={p.cap}
              type="button"
              className={`btn ${p.cls}`}
              disabled={busy}
              title={p.title}
              aria-pressed={maxConcurrent === p.cap}
              onClick={() => setMaxConcurrent(p.cap)}
            >
              {p.label}
            </button>
          ))}
        </div>
        <p className={styles.gateHint}>
          Measured at 3000 users vs 3000 seats — cap 10: 14.2s at a 2.6ms reserve p50. Cap 50: 4.8s
          at 25ms. Cap 3000: 3.2s at 1282ms. So the widest gate buys 4.4x the throughput for 500x the
          latency, and everything past ~50 is diminishing returns. Every setting sells exactly the
          capacity and never one seat more: correctness comes from the row lock, not the gate.
        </p>
      </div>

      <div className={tooSlow ? styles.estimateWarn : styles.estimate}>
        {tooSlow ? (
          <>
            <strong>This run will take {formatDuration(estimate)}.</strong> Admission is limited to{" "}
            {maxConcurrent} at a time, so {users.toLocaleString()} users drain at roughly{" "}
            {(users / estimate).toFixed(0)}/s. That is the gate working as designed, not a hang — raise the cap or
            lower the user count for a demo-length run.
          </>
        ) : (
          <>estimated run time {formatDuration(estimate)}</>
        )}
      </div>

      <div className={styles.actions}>
        <button className="btn btn-lime btn-block" disabled={!canRun} onClick={run}>
          {submitting ? "Starting…" : running ? "Running…" : "RUN SIMULATION"}
        </button>
        <button className="btn btn-pink btn-block" disabled={!running} onClick={onCancel}>
          CANCEL
        </button>
      </div>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
