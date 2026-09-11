import { useState } from "react";
import { api } from "../api/client";
import { ApiError, type EventItem, type ResetScope } from "../types/api";
import styles from "./SetupPanel.module.css";

// Number("") is 0 and Number("abc") is NaN; both serialise into a request the
// backend rejects with a 400 that looks like a bug in the demo.
function clampCount(raw: string, fallback: number, min: number, max: number): number {
  const parsed = Number(raw);
  if (raw.trim() === "" || Number.isNaN(parsed)) return fallback;
  return Math.min(Math.max(parsed, min), max);
}

function optionalCount(raw: string, fallback: number | ""): number | "" {
  if (raw.trim() === "") return "";
  const parsed = Number(raw);
  if (Number.isNaN(parsed) || parsed < 1) return fallback;
  return parsed;
}


interface SetupPanelProps {
  onEventsMinted: (events: EventItem[]) => void;
  onReset: () => void;
  disabled: boolean;
  collapsed: boolean;
  onToggle: () => void;
}

export function SetupPanel({ onEventsMinted, onReset, disabled, collapsed, onToggle }: SetupPanelProps) {
  // mint users
  const [userCount, setUserCount] = useState(500);
  const [userBusy, setUserBusy] = useState(false);
  const [userMsg, setUserMsg] = useState<string | null>(null);

  // mint events
  const [eventCount, setEventCount] = useState(1);
  const [eventCapacity, setEventCapacity] = useState<number | "">("");
  const [eventQuota, setEventQuota] = useState<number | "">("");
  const [eventBusy, setEventBusy] = useState(false);
  const [eventMsg, setEventMsg] = useState<string | null>(null);

  // reset
  const [scope, setScope] = useState<ResetScope>("reservations");
  const [confirmAll, setConfirmAll] = useState(false);
  const [resetBusy, setResetBusy] = useState(false);
  const [resetMsg, setResetMsg] = useState<string | null>(null);

  const handleMintUsers = async () => {
    setUserBusy(true);
    setUserMsg(null);
    try {
      const res = await api.mintUsers(userCount);
      setUserMsg(`Created ${res.created} / ${res.requested} users.`);
    } catch (err) {
      setUserMsg(err instanceof ApiError ? `Error: ${err.message}` : "Failed to mint users.");
    } finally {
      setUserBusy(false);
    }
  };

  const handleMintEvents = async () => {
    setEventBusy(true);
    setEventMsg(null);
    try {
      const res = await api.mintEvents(
        eventCount,
        eventCapacity === "" ? undefined : eventCapacity,
        eventQuota === "" ? undefined : eventQuota
      );
      setEventMsg(`Created ${res.created} / ${res.requested} events.`);
      onEventsMinted(res.events);
    } catch (err) {
      setEventMsg(err instanceof ApiError ? `Error: ${err.message}` : "Failed to mint events.");
    } finally {
      setEventBusy(false);
    }
  };

  const handleReset = async () => {
    if (scope === "all" && !confirmAll) {
      setConfirmAll(true);
      setResetMsg("This wipes ALL users, events and reservations. Click RESET again to confirm.");
      return;
    }
    setResetBusy(true);
    setResetMsg(null);
    try {
      const res = await api.reset(scope);
      setResetMsg(
        `Cleared: ${res.reservationsCleared} reservations, ${res.eventsCleared} events, ${res.usersCleared} users. Redis flushed: ${res.redisFlushed}.`
      );
      onReset();
    } catch (err) {
      setResetMsg(err instanceof ApiError ? `Error: ${err.message}` : "Failed to reset.");
    } finally {
      setResetBusy(false);
      setConfirmAll(false);
    }
  };

  return (
    <section className={`panel ${styles.panel}`}>
      <div className={styles.headRow}>
        <span className="panel-label">01 / setup</span>
        <button
          type="button"
          className={`btn ${styles.toggle}`}
          onClick={onToggle}
          aria-expanded={!collapsed}
          aria-controls="setup-body"
        >
          {collapsed ? "＋ mint / reset" : "− hide"}
        </button>
      </div>
      <div id="setup-body" hidden={collapsed} className={styles.grid}>
        <div className={styles.block}>
          <h3 className={styles.heading}>Mint Users</h3>
          <div className={styles.row}>
            <div className="field">
              <label htmlFor="userCount">Count</label>
              <input
                id="userCount"
                type="number"
                min={1}
                max={20000}
                value={userCount}
                disabled={disabled}
                onChange={(e) => setUserCount(clampCount(e.target.value, userCount, 1, 20000))}
              />
            </div>
            <button className="btn btn-lime" disabled={disabled || userBusy} onClick={handleMintUsers}>
              {userBusy ? "Minting..." : "Mint Users"}
            </button>
          </div>
          {userMsg && <p className={styles.msg}>{userMsg}</p>}
        </div>

        <div className={styles.block}>
          <h3 className={styles.heading}>Mint Events</h3>
          <div className={styles.row}>
            <div className="field">
              <label htmlFor="eventCount">Count</label>
              <input
                id="eventCount"
                type="number"
                min={1}
                max={500}
                value={eventCount}
                disabled={disabled}
                onChange={(e) => setEventCount(clampCount(e.target.value, eventCount, 1, 500))}
              />
            </div>
            <div className="field">
              <label htmlFor="eventCapacity">Capacity (opt)</label>
              <input
                id="eventCapacity"
                type="number"
                min={0}
                placeholder="auto"
                value={eventCapacity}
                disabled={disabled}
                onChange={(e) => setEventCapacity(optionalCount(e.target.value, eventCapacity))}
              />
            </div>
            <div className="field">
              <label htmlFor="eventQuota">Max/User (opt)</label>
              <input
                id="eventQuota"
                type="number"
                min={0}
                placeholder="auto"
                value={eventQuota}
                disabled={disabled}
                onChange={(e) => setEventQuota(optionalCount(e.target.value, eventQuota))}
              />
            </div>
            <button className="btn btn-lime" disabled={disabled || eventBusy} onClick={handleMintEvents}>
              {eventBusy ? "Minting..." : "Mint Events"}
            </button>
          </div>
          {eventMsg && <p className={styles.msg}>{eventMsg}</p>}
        </div>

        <div className={styles.block}>
          <h3 className={styles.heading}>Reset</h3>
          <div className={styles.row}>
            <div className="field">
              <label htmlFor="scope">Scope</label>
              <select
                id="scope"
                value={scope}
                disabled={disabled}
                onChange={(e) => {
                  setScope(e.target.value as ResetScope);
                  setConfirmAll(false);
                  setResetMsg(null);
                }}
              >
                <option value="reservations">reservations (keep users/events)</option>
                <option value="users">reservations + users (keep events)</option>
                <option value="all">all (wipe everything)</option>
              </select>
            </div>
            <button
              className={`btn ${scope === "all" ? "btn-pink" : "btn-blue"}`}
              disabled={disabled || resetBusy}
              onClick={handleReset}
            >
              {resetBusy ? "Resetting..." : confirmAll ? "Confirm Reset ALL" : "Reset"}
            </button>
          </div>
          <p className={styles.hint}>
            Every run mints one throwaway user per simulated user and never reuses them, so users
            pile up across runs. <strong>reservations + users</strong> clears that build-up without
            touching the events you set the demo up around.
          </p>
          {resetMsg && <p className={styles.msg}>{resetMsg}</p>}
        </div>
      </div>
    </section>
  );
}
