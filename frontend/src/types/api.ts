// ---- Core domain types, matching the backend API exactly ----

export interface EventItem {
  id: string;
  name: string;
  maxReservePerUser: number;
  capacity: number;
  createdAt: string;
  updatedAt: string;
}

export interface HealthResponse {
  postgres: boolean;
  redis: boolean;
}

export interface MintUsersResponse {
  requested: number;
  created: number;
  userIds: string[];
}

export interface MintEventsResponse {
  requested: number;
  created: number;
  events: EventItem[];
}

export type ResetScope = "reservations" | "users" | "all";

export interface ResetResponse {
  scope: ResetScope;
  reservationsCleared: number;
  eventsCleared: number;
  usersCleared: number;
  redisFlushed: boolean;
}

// "inproc" drives the service layer directly — no socket, no auth, no JSON — so it can
// run far more users than the machine has sockets. "http" goes through the real router
// and measures what a user would actually feel.
export type Transport = "inproc" | "http";

export interface SimulateRequest {
  eventId: string;
  users: number;
  capacity?: number;
  maxConcurrent?: number;
  quantity?: number;
  workers?: number;
  pollMs?: number;
  userTimeoutMs?: number;
  reset?: boolean;
  transport?: Transport;
}

export interface SimulateStartResponse {
  runId: string;
  eventId: string;
  users: number;
}

// ---- Snapshot: shape emitted by both the GET run endpoint and the SSE stream ----

export interface Snapshot {
  eventId: string;
  totalUsers: number;
  started: number;
  enqueued: number;
  admitted: number;
  reserved: number;
  rejectedCapacity: number;
  rejectedQuota: number;
  timedOut: number;
  cancelled: number;
  // Users whose heartbeat lapsed and were swept out of the queue. Zero in a healthy run;
  // non-zero means the client stopped polling, so it must be visible rather than silently
  // missing from the accounting.
  reaped: number;
  errors: number;
  inQueue: number;
  whitelistSize: number;
  seatsSold: number;
  capacity: number;
  capacityKnown: boolean;
  maxConcurrent: number;
  oversold: boolean;
  elapsedMs: number;
  done: boolean;
  reserveP50Ms: number;
  reserveP95Ms: number;
  reserveP99Ms: number;
  queueWaitP50Ms: number;
  queueWaitP95Ms: number;
  pollIntervalMs: number;
  workers: number;
  transport: Transport;
  pgAcquired: number;
  pgIdle: number;
  pgMax: number;
  redisTotalConns: number;
  redisIdleConns: number;
  redisTimeouts: number;
  errorCounts: Record<string, number>;
}

export interface ApiErrorBody {
  error: string;
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "ApiError";
  }
}
