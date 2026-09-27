import { ApiError, type ApiKey, type AuditEvent, type Me, type Profile, type Session, type UsageRow, type Worker } from "./types";

const ago = (min: number): string => new Date(Date.now() - min * 60000).toISOString();
const rid = (): string => Math.random().toString(36).slice(2, 10);
const secret = (p: string): string => p + Array.from({ length: 40 }, () => "abcdefghijklmnopqrstuvwxyz0123456789"[Math.floor(Math.random() * 36)]).join("");

let signedIn = true;

const me: Me = {
  user: { id: "u_1", email: "founder@northstar.dev", name: "Ayaan Rahman" },
  organization: { id: "org_1", name: "Any Endpoint" },
  role: "owner",
  csrf_token: "preview-csrf",
  environment: "preview",
};

const profiles: Profile[] = [];

const keys: ApiKey[] = [];

const usage: UsageRow[] = [];

const audit: AuditEvent[] = [];

const sessions: Session[] = [
  { id: "s1", device: "This browser", ip: "—", last_seen: ago(0), current: true },
];

const log = (type: string, category: AuditEvent["category"], target: string): void => {
  audit.unshift({ id: rid(), type, category, actor: me.user.email, target, created_at: new Date().toISOString() });
};

const workers = (): Worker[] =>
  profiles.map((p) => ({
    agent_id: "agt_" + p.id.slice(2, 10),
    profile_id: p.id,
    profile_label: p.label,
    provider: p.provider,
    state: p.worker_status,
    last_heartbeat: p.last_heartbeat,
    mtls: p.status === "disabled" ? "unverified" : "verified",
    child_ready: p.worker_status === "online",
  }));

const wait = (ms: number, signal?: AbortSignal): Promise<void> =>
  new Promise((res, rej) => {
    const t = setTimeout(res, ms);
    signal?.addEventListener("abort", () => {
      clearTimeout(t);
      rej(new DOMException("Aborted", "AbortError"));
    });
  });

const findProfile = (id: string): Profile => {
  const p = profiles.find((x) => x.id === id);
  if (!p) throw new ApiError(404, "Resource unavailable.", "not_found");
  return p;
};

/** In-memory preview backend mirroring the Control Plane API contract. */
export async function mockFetch(method: string, path: string, body: unknown, signal?: AbortSignal): Promise<unknown> {
  await wait(250 + Math.random() * 250, signal);
  const url = new URL(path, "http://x");
  const p = url.pathname;
  const b = (body ?? {}) as Record<string, unknown>;
  const m = (re: RegExp): RegExpMatchArray | null => p.match(re);

  if (p === "/api/me") {
    if (!signedIn) throw new ApiError(401, "Not signed in.", "unauthenticated");
    return me;
  }
  if (p === "/api/auth/logout" || p === "/api/auth/sessions/revoke_all") {
    signedIn = false;
    return { ok: true };
  }
  if (p === "/api/auth/preview_login") {
    signedIn = true;
    return { ok: true };
  }
  if (!signedIn) throw new ApiError(401, "Session expired.", "unauthenticated");

  if (p === "/api/operations/summary") {
    const recent = usage.filter((u) => Date.now() - new Date(u.created_at).getTime() < 864e5);
    return {
      profiles: profiles.length,
      active_keys: keys.filter((k) => k.status === "active").length,
      workers_online: profiles.filter((x) => x.worker_status === "online").length,
      workers_stale: profiles.filter((x) => x.worker_status === "stale").length,
      requests_24h: recent.length * 37,
      failures_24h: recent.filter((u) => u.status_code >= 400).length * 3,
    };
  }
  if (p === "/api/profiles" && method === "GET") return profiles;
  if (p === "/api/profiles" && method === "POST") {
    const prov = (b.provider as Profile["provider"]) ?? "claude";
    const np: Profile = {
      id: "p_" + rid(),
      label: String(b.label ?? "New profile"),
      provider: prov,
      allowed_models: (b.allowed_models as string[]) ?? [],
      status: "pending",
      worker_status: "offline",
      last_heartbeat: null,
      secret_ref_status: "missing",
    };
    profiles.push(np);
    log("profile.create", "profile", np.label);
    return np;
  }
  let r = m(/^\/api\/profiles\/([^/]+)$/);
  if (r) return findProfile(r[1]);
  r = m(/^\/api\/profiles\/([^/]+)\/(connect|reconnect|disable)$/);
  if (r) {
    const pr = findProfile(r[1]);
    log("profile." + r[2], "profile", pr.label);
    if (r[2] === "disable") {
      pr.status = "disabled";
      pr.worker_status = "offline";
      return pr;
    }
    pr.status = "active";
    return { enrollment_token: secret("nset_"), expires_at: new Date(Date.now() + 15 * 60000).toISOString() };
  }

  if (p === "/api/keys" && method === "GET") return keys;
  if (p === "/api/keys" && method === "POST") {
    const raw = secret("ns_live_");
    const k: ApiKey = {
      id: "k_" + rid(),
      name: String(b.name ?? "Untitled"),
      prefix: raw.slice(0, 12),
      type: (b.type as ApiKey["type"]) ?? "universal",
      grant_count: 1,
      expires_at: b.expires_days ? new Date(Date.now() + Number(b.expires_days) * 864e5).toISOString() : null,
      last_used_at: null,
      status: "active",
    };
    keys.unshift(k);
    log("key.create", "key", k.prefix);
    return { key: k, raw_key: raw };
  }
  r = m(/^\/api\/keys\/([^/]+)\/(rotate|revoke)$/);
  if (r) {
    const k = keys.find((x) => x.id === r![1]);
    if (!k) throw new ApiError(404, "Resource unavailable.", "not_found");
    if (k.status === "revoked") throw new ApiError(409, "Key already revoked.", "conflict");
    log("key." + r[2], "key", k.prefix);
    if (r[2] === "revoke") {
      k.status = "revoked";
      return k;
    }
    const raw = secret("ns_live_");
    k.prefix = raw.slice(0, 12);
    return { key: k, raw_key: raw };
  }

  if (p === "/api/usage") return usage.slice(0, Number(url.searchParams.get("limit") ?? 100));
  if (p === "/api/audit") return audit;
  if (p === "/api/workers") return workers();
  if (p === "/api/auth/sessions") return sessions;
  r = m(/^\/api\/auth\/sessions\/([^/]+)\/revoke$/);
  if (r) {
    const i = sessions.findIndex((s) => s.id === r![1] && !s.current);
    if (i >= 0) sessions.splice(i, 1);
    return { ok: true };
  }

  if (p === "/api/playground/runs") {
    const pr = findProfile(String(b.profile_id));
    if (pr.status === "disabled" || pr.worker_status === "offline") throw new ApiError(503, "The worker for this profile is offline.", "worker_unavailable");
    const prompt = String(b.prompt ?? "");
    const started = Date.now();
    await wait(900 + Math.random() * 900, signal);
    usage.unshift({ id: rid(), route: "/api/playground/runs", model: String(b.model), status_code: 200, latency_ms: Date.now() - started, created_at: new Date().toISOString(), profile_label: pr.label, key_prefix: null });
    log("playground.run", "playground", pr.label);
    return {
      id: "run_" + rid(),
      model: String(b.model),
      latency_ms: Date.now() - started,
      output: `Preview response from ${pr.label} (${String(b.model)}).\n\nYou asked:\n"${prompt.slice(0, 280)}"\n\nThis is demo output. Once preview mode is turned off, this panel shows the real model reply from your worker. It is always shown as plain text, so markup like <script> stays harmless.`,
    };
  }
  throw new ApiError(404, "Resource unavailable.", "not_found");
}
