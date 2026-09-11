# Load testing

The load generator is built into the backend rather than bolted on as a k6 script. It drives up to 20,000 concurrent users, streams progress over SSE, and reports a full accounting of where every user ended up.

**All numbers below were measured on one machine** — Apple Silicon laptop, Postgres and Redis in Docker, backend on the host, everything sharing the same CPU. Treat the *shapes* as the result and the absolute values as specific to that setup. The load generator competes with the system it measures, which matters most at the highest user counts.

---

## Running a test

### From the UI

http://localhost:5173 → pick an event → **RUN SIMULATION**. Presets for the gate (10 / 50 / 3000) sit next to the input, and finished runs accumulate in a comparison table.

### From the API

```bash
curl -s -X POST http://localhost:8000/admin/simulate \
  -H 'Content-Type: application/json' \
  -d '{
    "eventId": "<uuid>",
    "users": 20000,
    "capacity": 1000,
    "maxConcurrent": 10,
    "userTimeoutMs": 300000,
    "reset": true,
    "transport": "inproc"
  }'
# → {"runId":"...","eventId":"...","users":20000}

curl -s http://localhost:8000/admin/simulate/<runId>          # snapshot
curl -sN http://localhost:8000/admin/simulate/<runId>/stream  # live SSE
curl -s -X POST http://localhost:8000/admin/simulate/<runId>/cancel
```

One run per event at a time; a second returns `409` **before** touching any state.

### From the CLI

```bash
cd backend
go run ./cmd/simulate -event <uuid> -users 5000 -capacity 100 -cap 10
```

The CLI is in-process only. For HTTP transport use the API or the UI.

---

## Two transports

| | `inproc` | `http` |
|---|---|---|
| path | service layer called directly | real requests through the router |
| includes | Redis + Postgres only | socket, chi middleware, JWT, JSON |
| ceiling | 20,000 users | 4,000 users (sockets) |
| answers | "does the invariant hold under extreme concurrency?" | "what does a user actually experience?" |

Two transports rather than one because a single number would carry a hidden asterisk. `inproc` latency is the cost of the critical section — real, but not end-to-end. `http` latency is end-to-end but bounded by file descriptors. Each claim gets measured the way it should be.

Same workload, both transports (3,000 users, 100 seats, gate 50):

```
inproc   1.0s   p50  46.0ms   p95 126.5ms   p99 145.7ms
http     1.1s   p50 141.6ms   p95 274.9ms   p99 297.1ms
```

Simulated users authenticate with real JWTs minted using the same claims and secret as `/users/login`, so `http` exercises the genuine auth path.

---

## Reading the output

```
elapsed 35.2s  sold 1000/1000  oversold=False
admitted 1088  rejected:capacity 19000  rejected:quota 0
timed out 0  reaped 0  cancelled 0  errors 0
p50 3.12ms  p95 31.77ms  p99 113.50ms   pg 0/20  poll 400ms
```

| field | meaning |
|---|---|
| `oversold` | **the invariant.** `seatsSold > capacity`. Must always be false |
| accounting | `reserved + rejected:* + timedOut + cancelled + reaped + errors` should equal total users |
| `reaped` | heartbeat lapsed — the client stopped polling. Non-zero means the generator is starving |
| `pg` | pool connections at sample time, not a peak — see the note below |
| `poll` | auto-selected unless `pollMs` is given |

**`pg` reads low even under heavy load, and that is expected.** By Little's Law, in-flight connections equal arrival rate times holding time. A reserve transaction holds a connection for roughly a millisecond, so at 200 admissions/second the average is ~0.2 connections. Snapshots every 250ms catch 0 or 1. The pool is not a bottleneck in the gated configurations; it only becomes visible under `http`, where the per-request auth lookup pushes it to 18/20.

---

## What the gate buys

3,000 users against 3,000 seats — nobody is rejected, so wall clock means throughput:

| gate | wall clock | p50 | p95 | p99 | oversold |
|---:|---:|---:|---:|---:|:---:|
| 10 | 14.2s | 2.6ms | 20.5ms | 65.5ms | no |
| 50 | 4.8s | 25.4ms | 121.0ms | 191.8ms | no |
| 200 | 4.0s | 200.6ms | 353.3ms | 426.1ms | no |
| 1000 | 3.7s | 903.0ms | 1422.3ms | 1498.9ms | no |
| 3000 | 3.2s | 1281.5ms | 2367.2ms | 2475.9ms | no |

**4.4× the throughput for 500× the latency**, with the knee around 50. Over HTTP the same shape holds (2,000 users / 2,000 seats): gate 50 → 5.2s / 65.5ms, gate 200 → 2.9s / 185.2ms, gate 1000 → 2.4s / 521.6ms.

The `oversold` column is the point of the table. It never changes.

---

## The poll storm

The most useful thing the load test found, because it falsified the model the system was tuned against.

The working assumption was `admission rate = gate ÷ pollInterval`, predicting that a shorter poll interval finishes sooner. Measured at 20,000 users, gate 10, it is exactly backwards:

| poll interval | wall clock | reserve p50 | Redis ops/s (median) |
|---:|---:|---:|---:|
| 50ms | 142.4s | 1225.7ms | 151,689 |
| 100ms | 100.9s | 802.2ms | 163,755 |
| 200ms | 50.1s | 252.6ms | 157,459 |
| **400ms** | **34.9s** | **3.8ms** | 97,256 |
| 600ms | 53.3s | 3.1ms | 64,807 |
| 800ms | 64.2s | 2.3ms | 48,900 |
| 1200ms | 96.2s | 2.0ms | 32,653 |

A U-curve with a clear optimum. Redis saturates around 150–190k ops/s on this hardware, and past that point **the poll traffic queues ahead of the reserve path** — that is the 1.2-second reserve p50 at a 50ms interval. Below saturation, wall clock simply grows with how long a user takes to notice a change.

Holding the interval at 50ms and varying population shows the same ceiling from the other direction:

| users | wall clock | reserve p50 | Redis ops/s |
|---:|---:|---:|---:|
| 2,000 | 5.1s | 2.2ms | 54,468 |
| 5,000 | 11.9s | 51.8ms | 147,722 |
| 20,000 | 145.7s | 1256.7ms | 149,543 |

The general lesson is a property of poll-based queues, not of this simulator: **N waiting clients polling every T seconds is N/T background load competing with the work you care about.** It is the argument for exponential backoff, or for pushing admission over SSE/WebSocket instead of polling.

The generator now picks its interval from a **50,000 polls/second budget**, capped at 1s (`pollOpsBudget` in `simulation/engine.go`), which lands each population in the healthy regime:

| users | chosen interval | wall clock | reserve p50 |
|---:|---:|---:|---:|
| 2,000 | 50ms | 5.1s | 1.95ms |
| 5,000 | 100ms | 9.3s | 2.92ms |
| 20,000 | 400ms | 35.5s | 3.48ms |

The previous budget of 100,000 chose 200ms for 20,000 users — twice too aggressive, giving 50.1s and a 252ms p50.

---

## What the load test found

Bugs that only appeared under load, not code review:

**The queue wedge.** A failed reserve returned without releasing its admission slot. Once an event sold out, every waiting user inherited the same stuck slot: 302.7s against a 6.0s baseline. Introduced twice — once in the original error path, once when the blocked-event fast path was added.

**Silent reaping.** The generator never refreshed heartbeats, so at 30 seconds 16,192 users were swept out mid-run and simply vanished from the accounting. The first fix — a separate `Ping` call — made it worse by doubling Redis load. The correct fix merged both questions into one script.

**A stale sold-out marker.** `reset: true` on simulate cleared rows but not the LRU, so the first run looked perfect and every run after sold **zero seats** at `p50 = 0.00ms` — fast enough to read as a feature.

**Uncached auth.** `GetAuthenticatedUser` hits Postgres on every authenticated request; the Redis user cache exists but is unused. Same logical workload, both transports:

```
inproc   47 transactions    19,521 rows returned
http    634 transactions   101,123 rows returned      13.5× / 5.2×
```

Every poll over HTTP is a `SELECT` on `users`. This is the highest-leverage remaining fix.

---

## Regression suite

Run after any change to the queue, reserve path, or LRU:

| config | expected |
|---|---|
| 20,000 users / 1,000 seats / gate 10 / inproc | ~35s, 1000/1000, p50 ~3ms |
| 5,000 / 100 / gate 10 / inproc | ~1.5s, 100/100 |
| 3,000 / 3,000 / gate 50 / inproc | ~4s, 3000/3000 (never sells out) |
| 2,000 / 100 / gate 10 / http | ~2s, 100/100 |
| 2,000 / 2,000 / gate 200 / http | ~3s, 2000/2000 |
| 300 / 50 / gate 10 / http | ~0.5s, 50/50 |

Every row must show `oversold: false`, full accounting, and zero reaped, timed-out and errored users.

**Run them consecutively against the same event.** Several of the worst bugs only appear on the second and later runs, because they are cache-invalidation failures that a single clean run cannot expose.

---

## Housekeeping

Every run mints one throwaway user per simulated user and never reuses them, so users accumulate — roughly a million rows and 300 MB after a sweep session. Clear them without losing your events:

```bash
curl -s -X POST http://localhost:8000/admin/reset \
  -H 'Content-Type: application/json' -d '{"scope":"users"}'
```

Scopes are `reservations` (keep users and events), `users` (keep events), and `all`. The UI exposes all three.
