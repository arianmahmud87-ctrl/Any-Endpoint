import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ArrowLeft, AlertCircle, CheckCircle2, ExternalLink, LoaderCircle, Plus, XCircle } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useAction, useProfile, useProfiles } from "@/lib/api/hooks";
import { api } from "@/lib/api/client";
import type { Profile, Provider } from "@/lib/api/types";
import { Btn, Dot, Empty, ErrorState, Field, inputCls, LoadingRows, Page, Tag } from "@/components/kit";
import { Confirm } from "@/components/Dialogs";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { ProviderBadge, ProviderIcon } from "@/components/ProviderIcon";
import { relTime } from "@/lib/format";

const MODELS: Record<Provider, string[]> = { claude: ["sonnet", "opus", "haiku"], codex: ["gpt-5-codex", "gpt-5"] };
const INV = [["profiles"], ["summary"]];
const ACTIVE_LOGIN = ["pending", "awaiting_authorization", "validating"];
const TERMINAL_LOGIN = ["succeeded", "failed", "cancelled", "expired"];

interface LoginAttempt {
  id: string;
  profile_id: string;
  status: string;
  expires_at: string;
  authorization_url?: string;
  user_code?: string;
  challenge_expires_at?: string;
  failure_code?: string;
  failure_message?: string;
}

function StatePill({ state, tone = "neutral" }: { state: string; tone?: "neutral" | "good" | "warn" | "bad" }) {
  return <span className={cn("inline-flex items-center rounded-md px-2 py-1 text-[11.5px] font-medium capitalize", tone === "good" && "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300", tone === "warn" && "bg-amber-500/10 text-amber-700 dark:text-amber-300", tone === "bad" && "bg-destructive/10 text-destructive", tone === "neutral" && "bg-secondary text-muted-foreground")}>{state.replaceAll("_", " ")}</span>;
}

function provisioningTone(status: Profile["provisioning_status"]): "neutral" | "good" | "warn" | "bad" {
  if (status === "ready") return "good";
  if (status === "failed" || status === "expired") return "bad";
  if (status === "pending" || status === "claimed") return "warn";
  return "neutral";
}

function workerTone(status: Profile["worker_status"]): "neutral" | "good" | "warn" | "bad" {
  if (status === "online") return "good";
  if (status === "draining" || status === "stale") return "warn";
  if (status === "offline" || status === "not_enrolled") return "bad";
  return "neutral";
}

function actionForProfile(profile: Profile, onLogin: () => void, busy: boolean) {
  if (profile.status === "disabled") return <StatePill state="disabled" tone="neutral" />;
  if (profile.provisioning_status === "pending" || profile.provisioning_status === "claimed") return <Btn disabled><LoaderCircle className="h-3.5 w-3.5 animate-spin" />Starting worker…</Btn>;
  if (profile.provisioning_status === "failed" || profile.provisioning_status === "expired") return <StatePill state="setup failed" tone="bad" />;
  if (profile.provisioning_status === "not_configured") return <StatePill state="not configured" tone="neutral" />;
  if (profile.provider !== "codex") return <StatePill state="provider login unavailable" tone="neutral" />;
  if (profile.worker_status !== "online") return <Btn disabled><AlertCircle className="h-3.5 w-3.5" />Worker {profile.worker_status.replaceAll("_", " ")}</Btn>;
  return <Btn variant="primary" onClick={onLogin} disabled={busy}>{profile.status === "ready" ? "Reconnect Codex" : "Connect Codex"}</Btn>;
}

export function useProfileActions() {
  const [login, setLogin] = useState<LoginAttempt | null>(null);
  const [disabling, setDisabling] = useState<Profile | null>(null);
  const qc = useQueryClient();
  const startLogin = useAction<{ id: string }, { login: LoginAttempt }>((v) => `/api/profiles/${v.id}/login/start`, INV);
  const cancelLogin = useAction<{ id: string; attempt: string }, unknown>((v) => `/api/profiles/${v.id}/login/cancel?attempt_id=${encodeURIComponent(v.attempt)}`, INV);
  const disable = useAction<string>((id) => `/api/profiles/${id}/disable`, INV, { success: "Profile disabled" });
  const doLogin = (profile: Profile) => {
    if (profile.provider !== "codex" || profile.worker_status !== "online" || profile.status === "disabled") return;
    startLogin.mutate({ id: profile.id }, { onSuccess: (response) => setLogin(response.login) });
  };

  useEffect(() => {
    if (!login || TERMINAL_LOGIN.includes(login.status)) return;
    const timer = window.setInterval(async () => {
      try {
        const response = await api<{ login: LoginAttempt }>(`/api/profiles/${login.profile_id}/login/status?attempt_id=${encodeURIComponent(login.id)}`);
        setLogin(response.login);
        if (TERMINAL_LOGIN.includes(response.login.status)) qc.invalidateQueries({ queryKey: ["profiles"] });
      } catch {
        // Keep the current state visible across transient polling failures.
      }
    }, 2000);
    return () => window.clearInterval(timer);
  }, [login, qc]);

  const dialogs = (
    <>
      <Dialog open={!!login} onOpenChange={(open) => !open && setLogin(null)}>
        <DialogContent className="max-w-md rounded-2xl">
          <DialogHeader><DialogTitle className="text-base">Connect Codex</DialogTitle></DialogHeader>
          {login && <div className="space-y-4 text-[13px]">
            {login.status === "awaiting_authorization" && login.authorization_url && login.user_code ? <>
              <p className="text-muted-foreground">Open the authorization page and enter this one-time code. It stays in memory and expires automatically.</p>
              <a className="inline-flex items-center gap-1.5 font-medium underline" href={login.authorization_url} target="_blank" rel="noreferrer">Open Codex authorization <ExternalLink className="h-3.5 w-3.5" /></a>
              <div className="rounded-lg border bg-secondary p-3 text-center font-mono text-lg tracking-widest">{login.user_code}</div>
            </> : login.status === "succeeded" ? <div className="flex items-start gap-3 rounded-lg bg-emerald-500/10 p-3 text-emerald-700 dark:text-emerald-300"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" /><div><p className="font-medium">Codex connected</p><p className="mt-1 text-[12px]">The profile is ready to serve requests.</p></div></div> : TERMINAL_LOGIN.includes(login.status) ? <div className="flex items-start gap-3 rounded-lg bg-destructive/10 p-3 text-destructive"><XCircle className="mt-0.5 h-4 w-4 shrink-0" /><div><p className="font-medium">Codex connection {login.status}</p><p className="mt-1 text-[12px]">{login.failure_message || "No provider credential was stored in the browser."}</p></div></div> : <div className="flex items-center gap-3 rounded-lg bg-secondary p-3"><LoaderCircle className="h-4 w-4 animate-spin" /><div><p className="font-medium">{login.status === "validating" ? "Validating Codex access…" : "Waiting for worker…"}</p><p className="mt-1 text-[12px] text-muted-foreground">Keep this window open while the private worker completes the step.</p></div></div>}
            <div className="flex items-center justify-between"><StatePill state={login.status} tone={login.status === "succeeded" ? "good" : TERMINAL_LOGIN.includes(login.status) ? "bad" : "warn"} />{ACTIVE_LOGIN.includes(login.status) ? <Btn variant="danger" disabled={cancelLogin.isPending} onClick={() => cancelLogin.mutate({ id: login.profile_id, attempt: login.id }, { onSuccess: () => setLogin(null) })}>Cancel</Btn> : <Btn variant="primary" onClick={() => setLogin(null)}>Done</Btn>}</div>
          </div>}
        </DialogContent>
      </Dialog>
      <Confirm open={!!disabling} onOpenChange={(open) => !open && setDisabling(null)} title={`Disable ${disabling?.label ?? ""}?`} body="This stops the profile from serving requests. You can re-enable it through the worker provisioning flow later." confirmLabel="Disable" busy={disable.isPending} onConfirm={() => disabling && disable.mutate(disabling.id, { onSettled: () => setDisabling(null) })} />
    </>
  );
  return { doLogin, askDisable: setDisabling, connecting: startLogin.isPending, actionForProfile: (profile: Profile) => actionForProfile(profile, () => doLogin(profile), startLogin.isPending), dialogs };
}

function NewProfile({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [label, setLabel] = useState("");
  const [provider, setProvider] = useState<Provider>("codex");
  const [models, setModels] = useState<string[]>(MODELS.codex);
  const create = useAction<void>(() => "/api/profiles", INV, { body: () => ({ label, provider: provider === "claude" ? "claude_code" : provider, allowed_models: models }), success: "Profile created" });
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="max-w-md rounded-2xl"><DialogHeader><DialogTitle className="text-base">Add provider profile</DialogTitle></DialogHeader><form className="space-y-4" onSubmit={(event) => { event.preventDefault(); create.mutate(undefined, { onSuccess: () => { onOpenChange(false); setLabel(""); } }); }}><Field label="Profile name"><input className={inputCls} value={label} onChange={(event) => setLabel(event.target.value)} maxLength={60} placeholder="Codex — main" required /></Field><Field label="Provider"><div className="flex gap-2">{(["codex", "claude"] as Provider[]).map((item) => <button type="button" key={item} onClick={() => { setProvider(item); setModels(MODELS[item]); }} className={cn("flex h-10 flex-1 items-center justify-center gap-2 rounded-lg border text-[13px] capitalize", provider === item && "border-foreground bg-secondary font-medium")}><ProviderIcon provider={item} size={18} />{item}</button>)}</div></Field><Field label="Allowed models"><div className="flex flex-wrap gap-2">{MODELS[provider].map((model) => <button type="button" key={model} onClick={() => setModels((current) => current.includes(model) ? current.filter((value) => value !== model) : [...current, model])} className={cn("mono h-7 rounded-md border px-2 text-[12px]", models.includes(model) && "border-foreground bg-secondary")}>{model}</button>)}</div></Field><p className="text-[12px] text-muted-foreground">Your private worker is created automatically. Provider credentials never enter this browser.</p><div className="flex justify-end gap-2"><Btn type="button" onClick={() => onOpenChange(false)}>Cancel</Btn><Btn variant="primary" disabled={!label.trim() || !models.length || create.isPending}>{create.isPending ? "Creating…" : "Create profile"}</Btn></div></form></DialogContent></Dialog>;
}

function ProfileRow({ profile, onLogin, onDisable, busy }: { profile: Profile; onLogin: () => void; onDisable: () => void; busy: boolean }) {
  return <div className="flex flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3.5"><div className="flex min-w-[210px] flex-1 items-center gap-3"><ProviderIcon provider={profile.provider} size={32} /><div><div className="font-medium">{profile.label}</div><div className="mt-1 flex flex-wrap gap-1">{profile.allowed_models.map((model) => <Tag key={model}>{model}</Tag>)}</div></div></div><div className="w-28"><StatePill state={profile.status} tone={profile.status === "ready" ? "good" : profile.status === "disabled" ? "neutral" : "warn"} /></div><div className="w-32 space-y-1"><StatePill state={profile.provisioning_status} tone={provisioningTone(profile.provisioning_status)} /><div className="text-[11.5px] text-muted-foreground">{profile.worker_status.replaceAll("_", " ")} · {relTime(profile.last_heartbeat)}</div></div><div className="flex gap-1.5">{actionForProfile(profile, onLogin, busy)}{profile.status !== "disabled" && <Btn variant="danger" onClick={onDisable}>Disable</Btn>}</div></div>;
}

export default function Profiles() {
  const q = useProfiles();
  const [searchParams, setSearchParams] = useSearchParams();
  const [filter, setFilter] = useState<"all" | Provider>("all");
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();
  const actions = useProfileActions();
  useEffect(() => { if (searchParams.get("new")) { setCreating(true); setSearchParams({}, { replace: true }); } }, [searchParams, setSearchParams]);
  const rows = (q.data ?? []).filter((profile) => filter === "all" || profile.provider === filter);
  const needsAttention = rows.filter((profile) => profile.provisioning_status === "failed" || profile.provisioning_status === "expired" || profile.worker_status === "draining" || profile.worker_status === "offline").length;
  return <Page title="Provider profiles" subtitle="Private provider accounts for your workspace." actions={<Btn variant="primary" onClick={() => setCreating(true)}><Plus className="h-3.5 w-3.5" />Add profile</Btn>}>
    {needsAttention > 0 && <div className="mb-5 flex items-start gap-3 rounded-xl border border-amber-500/20 bg-amber-500/5 p-4 text-[13px]"><AlertCircle className="mt-0.5 h-4 w-4 text-amber-600" /><div><p className="font-medium">{needsAttention} profile{needsAttention === 1 ? " needs" : "s need"} attention</p><p className="mt-1 text-muted-foreground">Review the worker status below. Credentials and enrollment tokens are managed privately by the service.</p></div></div>}
    <div className="mb-4 flex gap-1">{(["all", "codex", "claude"] as const).map((item) => <button key={item} onClick={() => setFilter(item)} className={cn("inline-flex h-7 items-center gap-1.5 rounded-lg px-2.5 text-[13px] capitalize text-muted-foreground", filter === item && "bg-secondary font-medium text-foreground")}>{item !== "all" && <ProviderIcon provider={item} size={14} />}{item}</button>)}</div>
    {q.isLoading ? <LoadingRows /> : q.isError ? <ErrorState error={q.error} retry={q.refetch} /> : !rows.length ? <Empty title="No profiles yet" hint="Add a provider profile to start a private worker." action={<Btn variant="primary" onClick={() => setCreating(true)}>Add profile</Btn>} /> : <div className="divide-y rounded-xl border"><div className="hidden items-center gap-4 border-b bg-[hsl(var(--canvas))] px-4 py-2 text-[11.5px] font-medium uppercase tracking-wide text-muted-foreground md:flex"><span className="flex-1">Profile</span><span className="w-28">State</span><span className="w-32">Worker</span><span className="w-[220px] text-right">Action</span></div>{rows.map((profile) => <div key={profile.id} onDoubleClick={() => navigate(`/app/profiles/${profile.id}`)}><ProfileRow profile={profile} onLogin={() => actions.doLogin(profile)} onDisable={() => actions.askDisable(profile)} busy={actions.connecting} /></div>)}</div>}
    <NewProfile open={creating} onOpenChange={setCreating} />{actions.dialogs}
  </Page>;
}

export function ProfileDetail() {
  const { id = "" } = useParams();
  const query = useProfile(id);
  const actions = useProfileActions();
  const profile = query.data;
  return <div><div className="mx-auto max-w-5xl px-5 pt-6 md:px-10"><Link to="/app/profiles" className="inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"><RefreshCw className="h-3.5 w-3.5" />Profiles</Link></div>{query.isLoading ? <div className="mx-auto max-w-5xl p-10"><LoadingRows n={4} /></div> : query.isError || !profile ? <div className="mx-auto max-w-5xl p-10"><ErrorState error={query.error} retry={query.refetch} /></div> : <Page title={profile.label} subtitle={`${profile.provider} profile`} actions={<>{actions.actionForProfile(profile)}{profile.status !== "disabled" && <Btn variant="danger" onClick={() => actions.askDisable(profile)}>Disable</Btn>}</>}><div className="mb-5 flex flex-wrap items-center gap-2"><ProviderBadge provider={profile.provider} /><StatePill state={profile.status} tone={profile.status === "ready" ? "good" : profile.status === "disabled" ? "neutral" : "warn"} /><span className="text-[12px] text-muted-foreground">Updated {relTime(profile.last_heartbeat)}</span></div><dl className="divide-y rounded-xl border text-[13px]">{[["Provider", <ProviderBadge key="provider" provider={profile.provider} />],["Profile state", <StatePill key="state" state={profile.status} tone={profile.status === "ready" ? "good" : "warn"} />],["Worker provisioning", <StatePill key="provisioning" state={profile.provisioning_status} tone={provisioningTone(profile.provisioning_status)} />],["Worker", <StatePill key="worker" state={profile.worker_status} tone={workerTone(profile.worker_status)} />],["Last heartbeat", relTime(profile.last_heartbeat)],["Allowed models", <div key="models" className="flex flex-wrap gap-1">{profile.allowed_models.map((model) => <Tag key={model}>{model}</Tag>)}</div>]].map(([key, value]) => <div key={String(key)} className="flex items-center justify-between gap-4 px-4 py-3"><dt className="text-muted-foreground">{key}</dt><dd>{value}</dd></div>)}</dl></Page>}{actions.dialogs}</div>;
}
