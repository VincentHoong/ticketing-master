# API reference

Base URL in the default compose setup: `http://localhost:8000`.

Authentication is a bearer JWT: `Authorization: Bearer <token>`. Get one from `POST /users/login`. Routes marked 🔒 require it.

Errors are `{"error": "..."}`, with one exception: a `401` from the auth middleware returns an empty `{}`, because it serialises the error value rather than its message. Clients should treat `401` by status, not by body.

---

## Health

### `GET /health`

```json
{ "postgres": true, "redis": true }
```

---

## Users

### `POST /users` → `201`

```json
{ "name": "Ava Thompson", "email": "ava@example.com", "password": "..." }
```

### `POST /users/login` → `200`

The demo does not verify passwords — it issues a token for any known user id.

```json
// request
{ "id": "<user uuid>" }
// response
{ "token": "eyJhbGciOi..." }
```

### `GET /users/me` → `200` 🔒
### `GET /users/{id}` → `200`

---

## Events

### `GET /events` → `200`

Optional `?limit=N`.

```json
[
  {
    "id": "cccccccc-...",
    "name": "Jakarta Jazz Festival 2026",
    "capacity": 150,
    "maxReservePerUser": 6,
    "createdAt": "...",
    "updatedAt": "..."
  }
]
```

### `GET /events/{id}` → `200`
### `POST /events` → `201`

```json
{ "name": "...", "capacity": 200, "maxReservePerUser": 4, "startSalesAt": "2026-01-01T10:00:00Z" }
```

> `POST /events/bulk` and `DELETE /events/{id}` are registered but **not implemented** — the handlers are empty stubs.

---

## Virtual queue

All routes require auth. A client enqueues, polls until admitted, then reserves.

### `POST /virtual-queues/enqueue` → `201`

```json
// request
{ "eventId": "<uuid>" }
// response
{ "status": "whitelisted" }   // admitted immediately — go straight to reserve
{ "status": "in queue" }      // poll /ping until admitted
```

Calling it again while **already admitted** is safe and returns `201 whitelisted` — the call is idempotent for a holder of a slot. Calling it again while **still queued** returns `409 virtual queue already exist`.

### `POST /virtual-queues/ping` → `200`

The only polling endpoint. Answers "am I admitted?" and refreshes the heartbeat in one round trip — a client that is asking is by definition alive.

```json
// request
{ "eventId": "<uuid>" }
// response
{ "admitted": true,  "ttlSeconds": 28, "soldOut": false }
{ "admitted": false, "ttlSeconds": 0,  "soldOut": false }
{ "admitted": false, "ttlSeconds": 0,  "soldOut": true  }   // stop polling
```

`404` means neither whitelisted nor alive — the client was reaped, or never queued, and must enqueue again.

Poll roughly every 0.4–1s per waiting client. Shorter intervals cost more than they buy once the waiting population is large; see [load-testing.md](load-testing.md#the-poll-storm).

### `POST /virtual-queues/dequeue` → `201`

Leave the queue. Promotes the next user only if this actually freed an admission slot.

### `GET /virtual-queues/{eventId}/total` → `200`

```json
{ "total": 18432 }
```

---

## Reservations

### `POST /events/{id}/reserve` → `200` 🔒

The critical path. Requires an active admission slot.

```json
// request
{ "quantity": 2, "idempotencyKey": "optional-client-key" }
// response
{
  "id": "...", "eventId": "...", "userId": "...",
  "quantity": 2, "status": "held",
  "reservedAt": "...", "expiresAt": "..."
}
```

Holds expire after **10 minutes** and are swept back to `expired` by a background worker.

| status | meaning |
|---|---|
| `200` | reserved, or an idempotent replay of an identical earlier request |
| `400` | `not currently whitelisted for reservation` — no admission slot |
| `403` | idempotency key reused with a different quantity |
| `404` | event not found |
| `409` | `insufficient capacity` or `exceed maximum reserve quantity` |
| `504` | request deadline exceeded |

The two `409` reasons are kept distinct in the body: one is retryable in principle, the other never is.

Sending the same `idempotencyKey` twice returns the original reservation instead of a second one. Keys are scoped per `(event, user)`.

### `POST /reservations/{id}/confirm` → `201` 🔒

Hold → sold. Takes no request body: the reservation is identified by the path and the caller by their token.

The `UPDATE` guards on `status = 'held'` and a live `expires_at`, so it cannot double-sell or resurrect an expired hold.

**Replay-idempotent.** A repeated confirm returns `201` rather than an error. If the update matches no row, the reservation is re-read: when it is already `confirmed` *and owned by the caller*, that is a successful replay. No idempotency key is needed — a repeat is always the same operation.

| status | meaning |
|---|---|
| `201` | confirmed, or a replay of an earlier confirm by the same caller |
| `400` | not found, not owned by the caller, expired, or already released |

The ownership check matters: `400` is returned uniformly whether the reservation is missing, held by someone else, or confirmed by someone else, so the endpoint cannot be used to probe for reservation ids.

### `POST /reservations/{id}/release` → `201` 🔒

Manual release. Frees the seats and unblocks the event's sold-out marker.

> The brief specifies `DELETE /reservations/:id`. That route is registered but is an empty stub; release is the `POST` route above.

### `GET /reservations/{id}` → `200` 🔒

---

## Admin (demo only)

Registered **only when `DEMO_MODE=true`**. They truncate tables and flush Redis, and none of them require authentication. Never enable on a public deployment.

### `POST /admin/mint/users` → `201`

```json
{ "count": 500 }
→ { "requested": 500, "created": 500, "userIds": ["..."] }
```

Max 20,000.

### `POST /admin/mint/events` → `201`

```json
{ "count": 3, "capacity": 200, "maxReservePerUser": 4 }
```

`capacity` and `maxReservePerUser` are optional — omit for randomised values. Max 500.

### `POST /admin/reset` → `200`

```json
{ "scope": "users" }
→ { "scope":"users", "reservationsCleared":1000,
    "eventsCleared":0, "usersCleared":951305, "redisFlushed":true }
```

| scope | reservations | users | events |
|---|:---:|:---:|:---:|
| `reservations` (default) | ✓ | | |
| `users` | ✓ | ✓ | |
| `all` | ✓ | ✓ | ✓ |

Every scope flushes Redis and clears the sold-out markers. Use `users` for routine cleanup: each simulation mints a throwaway user per simulated user, so they accumulate fast.

> Rejects with `409` if any simulation is active — same guard `/admin/simulate` uses to protect its own run.

### `POST /admin/simulate` → `202`

Starts a load test. Returns immediately; poll or stream for progress.

```json
{
  "eventId": "<uuid>",       // required
  "users": 20000,            // required, 1..20000 (1..4000 for http)
  "capacity": 1000,          // optional: overwrite event capacity first
  "maxConcurrent": 10,       // optional: admission gate for this run
  "quantity": 1,             // seats per user, default 1
  "workers": 2000,           // optional: cap on concurrent reserve attempts
  "pollMs": 400,             // optional: omit to auto-select from a 50k ops/s budget
  "userTimeoutMs": 300000,   // optional, default 60000
  "reset": true,             // clear reservations, Redis and sold-out markers first
  "transport": "inproc"      // "inproc" (default) or "http"
}
→ { "runId": "...", "eventId": "...", "users": 20000 }
```

`409` if a run is already active for that event — checked **before** any state is mutated, so a rejected duplicate cannot damage the live run.

### `GET /admin/simulate/{runId}` → `200`

A snapshot. Same shape as each SSE frame:

```json
{
  "eventId": "...", "totalUsers": 20000, "started": 20000,
  "enqueued": 20000, "admitted": 1088, "reserved": 1000,
  "rejectedCapacity": 19000, "rejectedQuota": 0,
  "timedOut": 0, "cancelled": 0, "reaped": 0, "errors": 0,
  "inQueue": 0, "whitelistSize": 0,
  "seatsSold": 1000, "capacity": 1000, "capacityKnown": true,
  "maxConcurrent": 10, "oversold": false,
  "elapsedMs": 35200, "done": true,
  "reserveP50Ms": 3.12, "reserveP95Ms": 31.77, "reserveP99Ms": 113.50,
  "queueWaitP50Ms": 0, "queueWaitP95Ms": 0,
  "pollIntervalMs": 400, "workers": 2000, "transport": "inproc",
  "pgAcquired": 0, "pgIdle": 2, "pgMax": 20,
  "redisTotalConns": 42, "redisIdleConns": 40, "redisTimeouts": 0,
  "errorCounts": {}
}
```

`oversold` is the invariant. `reserved + rejected* + timedOut + cancelled + reaped + errors` should equal `totalUsers`.

### `GET /admin/simulate/{runId}/stream`

`text/event-stream`, one frame every 250ms until `done`. The write deadline is cleared for this route so a long run is not severed by the server's write timeout.

```
data: {"eventId":"...","reserved":412,...}
```

### `POST /admin/simulate/{runId}/cancel` → `202`

Runs are retained 10 minutes after completion, then forgotten.
