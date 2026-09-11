import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { HealthResponse } from "../types/api";

export interface HealthState {
  health: HealthResponse | null;
  reachable: boolean;
  checking: boolean;
}

/** Polls GET /health on an interval so the header can show a live status. */
export function useHealth(intervalMs = 5000): HealthState {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [reachable, setReachable] = useState(true);
  const [checking, setChecking] = useState(true);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;

    const check = async () => {
      try {
        const h = await api.health();
        if (!mounted.current) return;
        setHealth(h);
        setReachable(true);
      } catch {
        if (!mounted.current) return;
        setHealth(null);
        setReachable(false);
      } finally {
        if (mounted.current) setChecking(false);
      }
    };

    void check();
    const id = window.setInterval(check, intervalMs);
    return () => {
      mounted.current = false;
      window.clearInterval(id);
    };
  }, [intervalMs]);

  return { health, reachable, checking };
}
