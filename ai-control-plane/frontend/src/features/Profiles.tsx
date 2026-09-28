import { useEffect, useState } from "react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ArrowLeft, Plus } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useAction, useProfile, useProfiles, useWorkers } from "@/lib/api/hooks";
import { api } from "@/lib/api/client";
import type { Profile, Provider } from "@/lib/api/types";
import { Btn, Dot, Empty, ErrorState, Field, inputCls, LoadingRows, Page, Tag } from "@/components/kit";
import { Confirm, RevealOnce } from "@/components/Dialogs";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { ProviderBadge, ProviderIcon } from "@/components/ProviderIcon";
import { relTime } from "@/lib/format";

const MODELS: Record<Provider, string[]> = { claude: ["sonnet", "opus", "haiku"], codex: ["gpt-5-codex", "gpt-5"] };
const INV = [["profiles"], ["summary"], ["workers"]];

interface Enroll { enrollment_token: string; expires_at: string }
interface LoginAttempt { id: string; profile_id: string; status: string; expires_at: string; authorization_url?: string; user_code?: string; challenge_expires_at?: string }

/** Profile actions shared by list, detail and workers pages. */
export function useProfileActions() {
  const [token, setToken] = useState<Enroll | null>(null);
  const [login, setLogin] = useState<LoginAttempt | null>(null);
  const [disabling, setDisabling] = useState<Profile | null>(null);
  const qc = useQueryClient();
  const connect = useAction<{ id: string; kind: "connect" | "reconnect" }, Enroll>((v) => `/api/profiles/${v.id}/${v.kind}`, INV);
  const startLogin = useAction<{ id: string }, { login: LoginAttempt }>((v) => `/api/profiles/${v.id}/login/start`, INV);
  const cancelLogin = useAction<{ id: string; attempt: string }, unknown>((v) => `/api/profiles/${v.id}/login/cancel?attempt_id=${encodeURIComponent(v.attempt)}`, INV);
  const disable = useAction<string>((id) => `/api/profiles/${id}/disable`, INV, { success: "Profile disabled" });
  const doConnect = (p: Profile) =>
    connect.mutate({ id: p.id, kind: p.worker_status === "offline" && p.last_heartbeat === null ? "connect" : "reconnect" }, { onSuccess: (r) => { setToken(r); qc.invalidateQueries({ queryKey: ["profiles"] }); } });
  const doLogin = (p: Profile) => startLogin.mutate({ id: p.id }, { onSuccess: (r) => setLogin(r.login) });
  useEffect(() => {
    if (!login || ["succeeded", "failed", "cancelled", "expired"].includes(login.status)) return;
    const timer = window.setInterval(async () => {
      try {
        const response = await api<{ login: LoginAttempt }>(`/api/profiles/${login.profile_id}/login/status?attempt_id=${encodeURIComponent(login.id)}`);
        setLogin(response.login);
        if (["succeeded", "failed", "cancelled", "expired"].includes(response.login.status)) qc.invalidateQueries({ queryKey: ["profiles"] });
      } catch {
        // Keep the challenge visible while a transient status request fails.
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
            <p className="text-muted-foreground">Open the authorization page and enter the code below. This code is held in memory and expires automatically.</p>
            {login.authorization_url && <a className="break-all font-medium underline" href={login.authorization_url} target="_blank" rel="noreferrer">Open Codex authorization</a>}
            {login.user_code && <div className="rounded-lg border bg-secondary p-3 text-center font-mono text-lg tracking-widest">{login.user_code}</div>}
            <div className="flex items-center justify-between"><span className="capitalize text-muted-foreground">{login.status.replaceAll("_", " ")}</span>{["pending", "awaiting_authorization", "validating"].includes(login.status) ? <Btn variant="danger" disabled={cancelLogin.isPending} onClick={() => cancelLogin.mutate({ id: login.profile_id, attempt: login.id }, { onSuccess: () => setLogin(null) })}>Cancel</Btn> : <Btn variant="primary" onClick={() => setLogin(null)}>Done</Btn>}</div>
          </div>}
        </DialogContent>
      </Dialog>
      <RevealOnce
        open={!!token}
        onClose={() => setToken(null)}
        title="Worker enrollment token"
        value={token?.enrollment_token ?? ""}
        note="This token is for the worker agent only. It is not a provider credential."
        extra={token && <p className="text-[12px] text-muted-foreground">Expires {relTime(token.expires_at)}.</p>}
      />
      <Confirm
        open={!!disabling}
        onOpenChange={(o) => !o && setDisabling(null)}
        title={`Disable ${disabling?.label ?? ""}?`}
        body="The worker is disconnected and requests using this profile will fail until you reconnect it."
        confirmLabel="Disable"
        busy={disable.isPending}
        onConfirm={() => disabling && disable.mutate(disabling.id, { onSettled: () => setDisabling(null) })}
      />
    </>
  );
  return { doConnect, doLogin, askDisable: setDisabling, connecting: connect.isPending || startLogin.isPending, dialogs };
}

function NewProfile({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [label, setLabel] = useState<string>("");
  const [provider, setProvider] = useState<Provider>("claude");
  const [models, setModels] = useState<string[]>(MODELS.claude);
  const create = useAction<void>(() => "/api/profiles", INV, { body: () => ({ label, provider: provider === "claude" ? "claude_code" : provider, allowed_models: models }), success: "Profile created" });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md rounded-2xl">
        <DialogHeader><DialogTitle className="text-base">New provider profile</DialogTitle></DialogHeader>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); create.mutate(undefined, { onSuccess: () => { onOpenChange(false); setLabel(""); } }); }}>
          <Field label="Label"><input className={inputCls} value={label} onChange={(e) => setLabel(e.target.value)} maxLength={60} placeholder="Claude — main" required /></Field>
          <Field label="Provider">
            <div className="flex gap-2">
              {(["claude", "codex"] as Provider[]).map((p) => (
                <button type="button" key={p} onClick={() => { setProvider(p); setModels(MODELS[p]); }} className={cn("flex h-10 flex-1 items-center justify-center gap-2 rounded-lg border text-[13px] capitalize", provider === p && "border-foreground bg-secondary font-medium")}><ProviderIcon provider={p} size={18} />{p}</button>
              ))}
            </div>
          </Field>
          <Field label="Allowed models">
            <div className="flex flex-wrap gap-2">
              {MODELS[provider].map((m) => (
                <button type="button" key={m} onClick={() => setModels((s) => (s.includes(m) ? s.filter((x) => x !== m) : [...s, m]))} className={cn("mono h-7 rounded-md border px-2 text-[12px]", models.includes(m) && "border-foreground bg-secondary")}>{m}</button>
              ))}
            </div>
          </Field>
          <p className="text-[12px] text-muted-foreground">Your private worker is provisioned automatically. Provider credentials stay on the worker, not in this browser.</p>
          <div className="flex justify-end gap-2">
            <Btn type="button" onClick={() => onOpenChange(false)}>Cancel</Btn>
            <Btn variant="primary" disabled={!label.trim() || !models.length || create.isPending}>{create.isPending ? "Creating…" : "Create"}</Btn>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export default function Profiles() {
  const q = useProfiles();
  const [sp, setSp] = useSearchParams();
  const [filter, setFilter] = useState<"all" | Provider>("all");
  const [creating, setCreating] = useState<boolean>(false);
  const nav = useNavigate();
  const a = useProfileActions();
  const firstSetup = sp.get("first_setup") === "true";
  useEffect(() => { if (sp.get("new")) { setCreating(true); setSp({}, { replace: true }); } }, [sp, setSp]);
  const rows = (q.data ?? []).filter((p) => filter === "all" || p.provider === filter);
  return (
    <Page title="Provider profiles" subtitle="Codex and Claude accounts served by your private workers." actions={<Btn variant="primary" onClick={() => setCreating(true)}><Plus className="h-3.5 w-3.5" />Add profile</Btn>}>
      {firstSetup && <div className="mb-6 rounded-xl border bg-[hsl(var(--canvas))] p-4 text-[13px]"><span className="font-medium">Welcome.</span> Add your first profile. Your private worker will be provisioned automatically before you connect the provider.</div>}
      <div className="mb-4 flex gap-1">
        {(["all", "claude", "codex"] as const).map((f) => (
          <button key={f} onClick={() => setFilter(f)} className={cn("inline-flex h-7 items-center gap-1.5 rounded-lg px-2.5 text-[13px] capitalize text-muted-foreground", filter === f && "bg-secondary font-medium text-foreground")}>{f !== "all" && <ProviderIcon provider={f} size={14} />}{f}</button>
        ))}
      </div>
      {q.isLoading ? <LoadingRows /> : q.isError ? <ErrorState error={q.error} retry={q.refetch} /> : !rows.length ? (
        <Empty title="No profiles yet" hint="A profile links a provider account to a private worker." action={<Btn variant="primary" onClick={() => setCreating(true)}>Add profile</Btn>} />
      ) : (
        <div className="divide-y rounded-xl border">
          {rows.map((p) => (
            <div key={p.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
              <button className="flex min-w-[180px] flex-1 items-center gap-3 text-left" onClick={() => nav(`/app/profiles/${p.id}`)}>
                <ProviderIcon provider={p.provider} size={32} />
                <div>
                  <div className="font-medium">{p.label}</div>
                  <div className="mt-1 flex flex-wrap gap-1">{p.allowed_models.map((m) => <Tag key={m}>{m}</Tag>)}</div>
                </div>
              </button>
              <div className="w-24"><Dot state={p.status} /></div>
              <div className="w-32 text-[12.5px]"><Dot state={p.worker_status} /><div className="text-[11.5px] text-muted-foreground">{relTime(p.last_heartbeat)}</div></div>
              <div className="flex gap-1.5">
                {p.provisioning_status === "pending" || p.provisioning_status === "claimed" ? <Btn disabled>Provisioning…</Btn> : p.provisioning_status === "ready" && p.provider === "codex" ? <Btn onClick={() => a.doLogin(p)} disabled={a.connecting}>Connect Codex</Btn> : p.provisioning_status === "not_configured" && <Btn onClick={() => a.doConnect(p)} disabled={a.connecting}>{p.last_heartbeat ? "Reconnect" : "Connect"}</Btn>}
                {p.status !== "disabled" && <Btn variant="danger" onClick={() => a.askDisable(p)}>Disable</Btn>}
              </div>
            </div>
          ))}
        </div>
      )}
      <NewProfile open={creating} onOpenChange={setCreating} />
      {a.dialogs}
    </Page>
  );
}

export function ProfileDetail() {
  const { id = "" } = useParams();
  const q = useProfile(id);
  const w = useWorkers();
  const a = useProfileActions();
  const worker = w.data?.find((x) => x.profile_id === id);
  return (
    <div>
      <div className="mx-auto max-w-5xl px-5 pt-6 md:px-10"><Link to="/app/profiles" className="inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"><ArrowLeft className="h-3.5 w-3.5" />Profiles</Link></div>
      {q.isLoading ? <div className="mx-auto max-w-5xl p-10"><LoadingRows n={4} /></div> : q.isError ? <div className="mx-auto max-w-5xl p-10"><ErrorState error={q.error} retry={q.refetch} /></div> : (
        <Page title={q.data!.label} subtitle={`${q.data!.provider} profile`} actions={<>
          {q.data!.provisioning_status === "pending" || q.data!.provisioning_status === "claimed" ? <Btn disabled>Provisioning…</Btn> : q.data!.provisioning_status === "ready" && q.data!.provider === "codex" ? <Btn variant="primary" onClick={() => a.doLogin(q.data!)} disabled={a.connecting}>Connect Codex</Btn> : q.data!.provisioning_status === "not_configured" && <Btn onClick={() => a.doConnect(q.data!)}>{q.data!.last_heartbeat ? "Reconnect" : "Connect worker"}</Btn>}
          {q.data!.status !== "disabled" && <Btn variant="danger" onClick={() => a.askDisable(q.data!)}>Disable</Btn>}
        </>}>
          <dl className="divide-y rounded-xl border text-[13px]">
            {[
              ["Provider", <ProviderBadge key="p" provider={q.data!.provider} />],
              ["Status", <Dot key="s" state={q.data!.status} />],
              ["Allowed models", <div key="m" className="flex gap-1">{q.data!.allowed_models.map((m) => <Tag key={m}>{m}</Tag>)}</div>],
              ["Secret reference", <Dot key="r" state={q.data!.secret_ref_status === "bound" ? "verified" : "unverified"} label={q.data!.secret_ref_status} />],
              ["Worker provisioning", <Dot key="p" state={q.data!.provisioning_status} />],
              ["Worker", <Dot key="w" state={q.data!.worker_status} />],
              ["Agent", <span key="a" className="mono">{worker?.agent_id ?? "—"}</span>],
              ["Transport", worker ? <Dot key="t" state={worker.mtls} label={`mTLS ${worker.mtls}`} /> : "—"],
              ["Last heartbeat", relTime(q.data!.last_heartbeat)],
            ].map(([k, v], i) => (
              <div key={i} className="flex items-center justify-between px-4 py-3"><dt className="text-muted-foreground">{k}</dt><dd>{v}</dd></div>
            ))}
          </dl>
        </Page>
      )}
      {a.dialogs}
    </div>
  );
}
