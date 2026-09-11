Ticketing System — "Concert Seat Reservation"

The question to answer with code: When 500 concurrent requests hit the same 200-seat venue at the exact moment tickets go on sale, how do you guarantee zero overselling, zero deadlocks, and fair (not first-request-wins-by-luck) ordering — while keeping p99 latency reasonable?

Build this:

POST /events/:id/reserve — user requests N seats for an event. Must:
Never oversell (hard invariant — test this with a load script, not just trust your logic)
Return a reservation with a 5-minute hold (TTL), not an immediate purchase
Release the hold automatically if not confirmed
POST /reservations/:id/confirm — converts hold → sold, idempotently (retried requests must not double-charge or double-sell)
DELETE /reservations/:id — manual release
A virtual waiting room: when concurrent demand exceeds capacity, admit requests in controlled batches rather than letting all 500 hit your DB at once

Concurrency techniques to actually implement (don't just pick one — do at least two and compare):

Optimistic concurrency: UPDATE seats SET status='held' WHERE id=? AND status='available' — check rowCount, retry on conflict
Redis-based short-lived lock/reservation (SET seat:123 held EX 300 NX) — faster, but now you have two sources of truth to reconcile
Queue-based serialization: push all reservation attempts through a single Kafka partition per event, so one consumer resolves them strictly in order — no locking needed, contention resolved by architecture

Test/prove it: write a load-test script (k6 or plain Node with Promise.all) firing 1000 concurrent reservation requests at 200 seats. Your dashboard/logs should show exactly 200 succeed, the rest cleanly rejected, no partial/corrupt state. This artifact — a chart of your load test proving zero overselling — is your interview talking point.

Stretch: add a "flash sale" mode and measure how p99 latency changes as you swap approach 1 → 2 → 3 above. That comparison is the demonstration of understanding tradeoffs, not just implementing one.

ticketing teaches you contention-resolution (multiple techniques, pick based on tradeoffs)