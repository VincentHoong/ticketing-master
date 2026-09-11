import {
  ApiError,
  type ApiErrorBody,
  type EventItem,
  type HealthResponse,
  type MintEventsResponse,
  type MintUsersResponse,
  type ResetResponse,
  type ResetScope,
  type SimulateRequest,
  type SimulateStartResponse,
  type Snapshot,
} from "../types/api";

export const API_BASE: string =
  (import.meta.env.VITE_API_BASE as string | undefined) ?? "http://localhost:8000";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      headers: { "Content-Type": "application/json" },
      ...init,
    });
  } catch {
    throw new ApiError(0, `Network error: could not reach backend at ${API_BASE}`);
  }

  if (!res.ok) {
    let message = `Request failed with status ${res.status}`;
    try {
      const body = (await res.json()) as ApiErrorBody;
      if (body?.error) message = body.error;
    } catch {
      // ignore parse failure, keep default message
    }
    throw new ApiError(res.status, message);
  }

  const body = await res.text();
  if (body === "") {
    return undefined as T;
  }

  return JSON.parse(body) as T;
}

export const api = {
  health: () => request<HealthResponse>("/health"),

  listEvents: (limit = 100) => request<EventItem[]>(`/events?limit=${limit}`),

  mintUsers: (count: number) =>
    request<MintUsersResponse>("/admin/mint/users", {
      method: "POST",
      body: JSON.stringify({ count }),
    }),

  mintEvents: (count: number, capacity?: number, maxReservePerUser?: number) =>
    request<MintEventsResponse>("/admin/mint/events", {
      method: "POST",
      body: JSON.stringify({
        count,
        ...(capacity ? { capacity } : {}),
        ...(maxReservePerUser ? { maxReservePerUser } : {}),
      }),
    }),

  reset: (scope: ResetScope) =>
    request<ResetResponse>("/admin/reset", {
      method: "POST",
      body: JSON.stringify({ scope }),
    }),

  simulate: (req: SimulateRequest) =>
    request<SimulateStartResponse>("/admin/simulate", {
      method: "POST",
      body: JSON.stringify(req),
    }),

  getRun: (runId: string) => request<Snapshot>(`/admin/simulate/${runId}`),

  cancelRun: (runId: string) =>
    request<void>(`/admin/simulate/${runId}/cancel`, { method: "POST" }),

  streamUrl: (runId: string) => `${API_BASE}/admin/simulate/${runId}/stream`,
};
