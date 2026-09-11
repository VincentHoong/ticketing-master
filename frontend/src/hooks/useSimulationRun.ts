import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { ApiError, type Snapshot, type SimulateRequest } from "../types/api";

const RUN_ID_KEY = "ticketing-demo:runId";
const HISTORY_CAP = 400;
const MAX_STREAM_RETRIES = 5;
const RETRY_DELAYS_MS = [500, 1000, 2000, 4000, 8000];
const RECONNECT_MESSAGE = "Lost the live stream — reconnecting…";

export interface CompletedRun {
  runId: string;
  snapshot: Snapshot;
  finishedAt: number;
  pgPeak: number;
  redisPeak: number;
}

export interface SimulationRunState {
  runId: string | null;
  snapshot: Snapshot | null;
  history: Snapshot[];
  running: boolean;
  reconnecting: boolean;
  error: string | null;
  start: (req: SimulateRequest) => Promise<void>;
  cancel: () => Promise<void>;
  completedRuns: CompletedRun[];
  clearError: () => void;
}

export function useSimulationRun(): SimulationRunState {
  const [runId, setRunId] = useState<string | null>(() => {
    try {
      return localStorage.getItem(RUN_ID_KEY);
    } catch {
      return null;
    }
  });
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [history, setHistory] = useState<Snapshot[]>([]);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [completedRuns, setCompletedRuns] = useState<CompletedRun[]>([]);
  const [reconnecting, setReconnecting] = useState(false);
  const esRef = useRef<EventSource | null>(null);
  const lastRunIdRecorded = useRef<string | null>(null);
  const retryRef = useRef<number | null>(null);
  const attemptsRef = useRef(0);
  const doneRef = useRef(false);

  const closeStream = useCallback(() => {
    if (retryRef.current !== null) {
      window.clearTimeout(retryRef.current);
      retryRef.current = null;
    }
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
  }, []);

  const applySnapshot = useCallback((snap: Snapshot, id: string) => {
    doneRef.current = snap.done;
    setSnapshot(snap);
    setHistory((prev) => {
      const next = [...prev, snap];
      return next.length > HISTORY_CAP ? next.slice(next.length - HISTORY_CAP) : next;
    });
    setRunning(!snap.done);
    if (snap.done && lastRunIdRecorded.current !== id) {
      lastRunIdRecorded.current = id;
      setHistory((hist) => {
        const pgPeak = hist.reduce((m, h) => Math.max(m, h.pgAcquired), snap.pgAcquired);
        const redisPeak = hist.reduce(
          (m, h) => Math.max(m, h.redisTotalConns - h.redisIdleConns),
          snap.redisTotalConns - snap.redisIdleConns
        );
        setCompletedRuns((prev) =>
          [{ runId: id, snapshot: snap, finishedAt: Date.now(), pgPeak, redisPeak }, ...prev].slice(0, 5)
        );
        return hist;
      });
    }
  }, []);

  const openStream = useCallback(
    (id: string) => {
      closeStream();
      const es = new EventSource(api.streamUrl(id));
      esRef.current = es;
      es.onmessage = (ev) => {
        try {
          const snap = JSON.parse(ev.data) as Snapshot;
          attemptsRef.current = 0;
          setReconnecting(false);
          setError((prev) => (prev === RECONNECT_MESSAGE ? null : prev));
          applySnapshot(snap, id);
          if (snap.done) {
            closeStream();
          }
        } catch {
          // ignore malformed payloads
        }
      };
      // The run keeps going server-side when a stream drops, so reconnect rather than
      // stranding the UI: cancel would be disabled and a fresh run would 409.
      es.onerror = () => {
        closeStream();
        if (doneRef.current) return;

        if (attemptsRef.current >= MAX_STREAM_RETRIES) {
          setReconnecting(false);
          setRunning(false);
          setError(
            "Lost connection to the live stream and could not reconnect. The run may still be in progress on the server."
          );
          return;
        }

        const delay = RETRY_DELAYS_MS[Math.min(attemptsRef.current, RETRY_DELAYS_MS.length - 1)];
        attemptsRef.current += 1;
        setReconnecting(true);
        setError(RECONNECT_MESSAGE);
        retryRef.current = window.setTimeout(() => openStream(id), delay);
      };
    },
    [applySnapshot, closeStream]
  );

  // Recover a run in progress after page refresh.
  useEffect(() => {
    if (!runId) return;
    let cancelled = false;
    (async () => {
      try {
        const snap = await api.getRun(runId);
        if (cancelled) return;
        setSnapshot(snap);
        setHistory([snap]);
        setRunning(!snap.done);
        doneRef.current = snap.done;
        if (!snap.done) {
          openStream(runId);
        }
      } catch {
        if (cancelled) return;
        // Run no longer exists on the server (restart, TTL, etc). Forget it.
        try {
          localStorage.removeItem(RUN_ID_KEY);
        } catch {
          /* ignore */
        }
        setRunId(null);
      }
    })();
    return () => {
      cancelled = true;
    };
    // Only run on mount / when runId first hydrates from storage.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => closeStream, [closeStream]);

  const start = useCallback(
    async (req: SimulateRequest) => {
      setError(null);
      closeStream();
      try {
        const res = await api.simulate(req);
        setRunId(res.runId);
        try {
          localStorage.setItem(RUN_ID_KEY, res.runId);
        } catch {
          /* ignore */
        }
        setSnapshot(null);
        setHistory([]);
        setRunning(true);
        doneRef.current = false;
        attemptsRef.current = 0;
        setReconnecting(false);
        openStream(res.runId);
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) {
          setError("A simulation is already running for this event. Cancel it first or wait for it to finish.");
        } else if (err instanceof ApiError) {
          setError(err.message);
        } else {
          setError("Failed to start simulation.");
        }
        setRunning(false);
      }
    },
    [closeStream, openStream]
  );

  const cancel = useCallback(async () => {
    if (!runId) return;
    try {
      await api.cancelRun(runId);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      }
    }
  }, [runId]);

  const clearError = useCallback(() => setError(null), []);

  return { runId, snapshot, history, running, reconnecting, error, start, cancel, completedRuns, clearError };
}
