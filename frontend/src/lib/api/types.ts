export type Provider = "codex" | "claude";
export type Role = "owner" | "admin" | "member";
export type WorkerState = "online" | "stale" | "offline";

export interface Me {
  user: { id: string; email: string; name: string; avatar_url?: string };
  organization: { id: string; name: string };
  role: Role;
  csrf_token: string;
  environment: string;
}

export interface Profile {
  id: string;
  label: string;
  provider: Provider;
  allowed_models: string[];
  status: "active" | "pending" | "disabled";
  worker_status: WorkerState;
  last_heartbeat: string | null;
  secret_ref_status: "bound" | "missing";
}

export interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  type: "codex" | "claude" | "universal";
  grant_count: number;
  expires_at: string | null;
  last_used_at: string | null;
  status: "active" | "revoked";
}

export interface UsageRow {
  id: string;
  route: string;
  model: string;
  status_code: number;
  latency_ms: number;
  created_at: string;
  profile_label: string;
  key_prefix: string | null;
}

export interface AuditEvent {
  id: string;
  type: string;
  category: "auth" | "profile" | "key" | "worker" | "playground" | "denied";
  actor: string;
  target: string;
  created_at: string;
}

export interface Worker {
  agent_id: string;
  profile_id: string;
  profile_label: string;
  provider: Provider;
  state: WorkerState;
  last_heartbeat: string | null;
  mtls: "verified" | "unverified";
  child_ready: boolean;
}

export interface Summary {
  profiles: number;
  active_keys: number;
  workers_online: number;
  workers_stale: number;
  requests_24h: number;
  failures_24h: number;
}

export interface Session {
  id: string;
  device: string;
  ip: string;
  last_seen: string;
  current: boolean;
}

export interface PlaygroundRun {
  id: string;
  output: string;
  model: string;
  latency_ms: number;
}

export class ApiError extends Error {
  status: number;
  type: string;
  retryAfter?: number;
  constructor(status: number, message: string, type = "error", retryAfter?: number) {
    super(message);
    this.status = status;
    this.type = type;
    this.retryAfter = retryAfter;
  }
}
