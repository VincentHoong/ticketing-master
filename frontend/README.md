# Ticketing Master Demo (Frontend)

A React + TypeScript + Vite frontend that is the proof artifact for the ticketing system's
concurrency design: run a load test of thousands of simulated users hitting a fixed-capacity
event at once, and watch — live — that seats sold never exceeds capacity, no matter how many
requests land at the same instant.

## Running it

Requires the backend to be running **with `DEMO_MODE=true`** at `http://localhost:8000`
(see `../backend`). The demo endpoints (`/admin/mint/*`, `/admin/simulate*`, `/admin/reset`)
only exist in demo mode.

```bash
cd frontend
npm install
cp .env.example .env   # optional, defaults to http://localhost:8000 already
npm run dev
```

Then open the printed local URL (usually `http://localhost:5173`).

To point at a different backend URL, set `VITE_API_BASE` in `.env`:

```
VITE_API_BASE=http://localhost:8000
```

### Build

```bash
npm run build
```

Type-checks with `tsc -b` and produces a production build with Vite in `dist/`.

## What this demo proves

Under the hood, users first join a Redis-backed virtual queue; a background promoter admits a
capped number of them (`maxConcurrent`) into a whitelist, and only whitelisted users may attempt
to reserve a row-locked seat in Postgres. This is admission control: it protects the database
from a stampede. The UI's whole argument is visible in one screen — set `maxConcurrent` low
("Gate On") and the whitelist chart stays pinned flat while the queue drains, DB latency stays
low, and `reserveP95` is small; set it absurdly high ("Gate Off") and the stampede hits Postgres
directly, latency degrades roughly an order of magnitude — but in both cases the hero number,
`seatsSold / capacity`, holds exactly at capacity and never overshoots. Zero overselling, with or
without the gate; the gate is what keeps it fast.

## Notes / known gaps

- The UI has not been exercised against a live backend in this environment (no backend was
  running here) — it was built defensively: unreachable API, malformed SSE payloads, and 409
  "already running" are all handled and surfaced in the UI, but the actual live data flow
  (SSE stream shape, timing) has not been visually verified end-to-end.
- Page refresh mid-run recovery relies on `GET /admin/simulate/{runId}` returning an error for
  unknown/expired run ids; any error response is treated as "forget this run" and clears local
  storage.
