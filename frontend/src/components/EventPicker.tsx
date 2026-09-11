import type { EventItem } from "../types/api";
import styles from "./EventPicker.module.css";

interface EventPickerProps {
  events: EventItem[];
  selectedId: string | null;
  onSelect: (event: EventItem) => void;
  loading: boolean;
  error: string | null;
  onRefresh: () => void;
}

export function EventPicker({ events, selectedId, onSelect, loading, error, onRefresh }: EventPickerProps) {
  return (
    <section className={`panel ${styles.panel}`}>
      <div className={styles.headRow}>
        <span className="panel-label">02 / choose event</span>
        <button className="btn" onClick={onRefresh} disabled={loading}>
          {loading ? "Loading..." : "Refresh"}
        </button>
      </div>

      {error && <p className={styles.error}>Could not load events: {error}</p>}

      {!error && events.length === 0 && !loading && (
        <p className={styles.empty}>No events yet. Mint some above.</p>
      )}

      <div className={styles.grid}>
        {events.map((ev) => {
          const active = ev.id === selectedId;
          return (
            <button
              key={ev.id}
              title={ev.id}
              className={`${styles.card} ${active ? styles.active : ""}`}
              onClick={() => onSelect(ev)}
              type="button"
            >
              <div className={styles.name}>{ev.name}</div>
              <div className={styles.metaRow}>
                <span className="tag">CAP {ev.capacity}</span>
                <span className="tag">MAX/USER {ev.maxReservePerUser}</span>
              </div>
            </button>
          );
        })}
      </div>
    </section>
  );
}
