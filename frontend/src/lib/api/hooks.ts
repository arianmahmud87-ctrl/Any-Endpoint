import { useMutation, useQuery, useQueryClient, type QueryKey } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, errorMessage } from "./client";
import type { ApiKey, Me, Profile, Session, Summary, UsageRow, Worker } from "./types";

const unwrapList = <T,>(value: unknown): T[] => Array.isArray(value) ? value as T[] : ((value as { data?: T[] })?.data ?? []);

const normalizeMe = (raw: any): Me => ({
  user: { id: raw.user?.id ?? "", email: raw.user?.email ?? "", name: raw.user?.name ?? raw.user?.email?.split("@")[0] ?? "User", avatar_url: raw.user?.avatar_url },
  organization: { id: raw.organization?.id ?? "", name: raw.organization?.name ?? "Personal workspace" },
  role: raw.role ?? raw.organization?.role ?? "member",
  csrf_token: raw.csrf_token ?? "",
  environment: import.meta.env.MODE,
});

const normalizeProfile = (raw: any): Profile => ({
  id: raw.id ?? raw.ID,
  label: raw.label ?? raw.Label,
  provider: (raw.provider ?? raw.Provider) === "claude_code" ? "claude" : (raw.provider ?? raw.Provider),
  allowed_models: raw.allowed_models ?? raw.AllowedModels ?? [],
  status: raw.status ?? raw.Status,
  worker_status: raw.worker_status ?? raw.WorkerStatus ?? "offline",
  last_heartbeat: raw.last_heartbeat ?? raw.LastHeartbeat ?? null,
  secret_ref_status: raw.secret_ref_status ?? "bound",
});

const normalizeKey = (raw: any): ApiKey => ({
  id: raw.id,
  name: raw.name,
  prefix: raw.prefix,
  type: raw.type,
  grant_count: raw.grant_count ?? raw.grants?.length ?? 0,
  expires_at: raw.expires_at ?? null,
  last_used_at: raw.last_used_at ?? null,
  status: raw.status ?? raw.state ?? "active",
});

const normalizeUsage = (raw: any): UsageRow => ({
  id: raw.id ?? `${raw.created_at}-${raw.route}`,
  route: raw.route,
  model: raw.model ?? "",
  status_code: raw.status_code,
  latency_ms: raw.latency_ms ?? 0,
  created_at: raw.created_at,
  profile_label: raw.provider_profile_id ?? "session",
  key_prefix: raw.key_id ?? null,
});

const normalizeSummary = (raw: any): Summary => ({
  profiles: raw.profiles?.total ?? 0,
  active_keys: raw.keys?.active ?? 0,
  workers_online: raw.workers?.online ?? 0,
  workers_stale: raw.workers?.stale ?? 0,
  requests_24h: raw.last_24_hours?.requests ?? 0,
  failures_24h: raw.last_24_hours?.failures ?? 0,
});

export const useMe = () => useQuery({ queryKey: ["me"], queryFn: async () => normalizeMe(await api<any>("/api/me")), retry: false, staleTime: 60000 });
export const useSummary = () => useQuery({ queryKey: ["summary"], queryFn: async () => normalizeSummary(await api<any>("/api/operations/summary")) });
export const useProfiles = () => useQuery({ queryKey: ["profiles"], queryFn: async () => unwrapList<any>(await api<any>("/api/profiles")).map(normalizeProfile) });
export const useProfile = (id: string) => useQuery({ queryKey: ["profiles", id], queryFn: async () => { const raw = await api<any>(`/api/profiles/${id}`); return normalizeProfile(raw.profile ?? raw); } });
export const useKeys = () => useQuery({ queryKey: ["keys"], queryFn: async () => unwrapList<any>(await api<any>("/api/keys")).map(normalizeKey) });
export const useUsage = (limit = 100) => useQuery({ queryKey: ["usage", limit], queryFn: async () => unwrapList<any>(await api<any>(`/api/usage?limit=${limit}`)).map(normalizeUsage) });
export const useWorkers = () => useQuery<Worker[]>({ queryKey: ["workers"], queryFn: async () => (await useProfilesSnapshot()).map((p) => ({ agent_id: `agent_${p.id}`, profile_id: p.id, profile_label: p.label, provider: p.provider, state: p.worker_status as Worker["state"], last_heartbeat: p.last_heartbeat, mtls: "unverified", child_ready: p.worker_status === "online" })) });
async function useProfilesSnapshot(): Promise<Profile[]> { return unwrapList<any>(await api<any>("/api/profiles")).map(normalizeProfile); }
export const useSessions = () => useQuery<Session[]>({ queryKey: ["sessions"], queryFn: async () => [] });

export function useAction<TVars, TRes = unknown>(toPath: (v: TVars) => string, invalidate: QueryKey[], opts: { body?: (v: TVars) => unknown; success?: string } = {}) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: TVars) => api<TRes>(toPath(v), { method: "POST", body: opts.body?.(v) ?? {} }),
    onSuccess: () => { invalidate.forEach((k) => qc.invalidateQueries({ queryKey: k })); if (opts.success) toast(opts.success); },
    onError: (e) => { toast.error(errorMessage(e)); invalidate.forEach((k) => qc.invalidateQueries({ queryKey: k })); },
  });
}
