import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api/client";
import { useHealth } from "./hooks/useHealth";
import { useSimulationRun } from "./hooks/useSimulationRun";
import type { EventItem } from "./types/api";
import { Header } from "./components/Header";
import { SetupPanel } from "./components/SetupPanel";
import { EventPicker } from "./components/EventPicker";
import { SimulationControls } from "./components/SimulationControls";
import { LiveMonitor } from "./components/LiveMonitor/LiveMonitor";
import { RunComparison } from "./components/RunComparison";
import styles from "./App.module.css";

function App() {
  const healthState = useHealth();
  const { runId, snapshot, history, running, reconnecting, error, start, cancel, completedRuns, clearError } =
    useSimulationRun();

  const [events, setEvents] = useState<EventItem[]>([]);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [selectedEvent, setSelectedEvent] = useState<EventItem | null>(null);
  const [setupCollapsed, setSetupCollapsed] = useState(true);
  const loadedOnce = useRef(false);
  const monitorRef = useRef<HTMLDivElement | null>(null);

  const loadEvents = useCallback(async () => {
    setEventsLoading(true);
    setEventsError(null);
    try {
      const res = await api.listEvents(100);
      setEvents(res);
      // Only after a completed fetch can an empty list mean "there is nothing to run
      // against"; before that it just means the request has not returned yet.
      if (res.length === 0) setSetupCollapsed(false);
    } catch (err) {
      setEventsError(err instanceof Error ? err.message : "Failed to load events.");
    } finally {
      loadedOnce.current = true;
      setEventsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadEvents();
  }, [loadEvents]);

  const handleEventsMinted = (minted: EventItem[]) => {
    setEvents((prev) => [...minted, ...prev]);
  };

  const handleReset = () => {
    setSelectedEvent(null);
    void loadEvents();
  };

  return (
    <div className={styles.app}>
      <div className={styles.container}>
        <Header {...healthState} />

        <main>
          <SetupPanel
            onEventsMinted={handleEventsMinted}
            onReset={handleReset}
            disabled={running}
            collapsed={setupCollapsed}
            onToggle={() => setSetupCollapsed((c) => !c)}
          />

          <EventPicker
            events={events}
            selectedId={selectedEvent?.id ?? null}
            onSelect={setSelectedEvent}
            loading={eventsLoading}
            error={eventsError}
            onRefresh={loadEvents}
          />

          <SimulationControls
            selectedEvent={selectedEvent}
            running={running}
            onRun={async (req) => {
              clearError();
              await start(req);
              monitorRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
            }}
            onCancel={() => void cancel()}
            error={error}
          />

          <div ref={monitorRef}>
            <LiveMonitor snapshot={snapshot} history={history} running={running} reconnecting={reconnecting} />
          </div>

          <RunComparison runs={completedRuns} />
        </main>

        <footer className={styles.footer}>
          <span>ticketing master demo</span>
          {runId && <span className="mono">run {runId.slice(0, 8)}</span>}
        </footer>
      </div>
    </div>
  );
}

export default App;
