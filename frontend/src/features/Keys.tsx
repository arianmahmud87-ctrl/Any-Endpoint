import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Plus } from "lucide-react";
import { useAction, useKeys, useProfiles } from "@/lib/api/hooks";
import type { ApiKey } from "@/lib/api/types";
import { Btn, Dot, Empty, ErrorState, Field, inputCls, LoadingRows, Page, Table, Tag } from "@/components/kit";
import { Confirm, RevealOnce } from "@/components/Dialogs";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { relTime } from "@/lib/format";

interface Created { key: string; metadata?: ApiKey; warning?: string }
const INV = [["keys"], ["summary"]];

export default function Keys() {
  const q = useKeys();
  const profiles = useProfiles();
  const [sp, setSp] = useSearchParams();
  const [creating, setCreating] = useState<boolean>(false);
  const [name, setName] = useState<string>("");
  const [type, setType] = useState<ApiKey["type"]>("universal");
  const [days, setDays] = useState<string>("90");
  const [raw, setRaw] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<{ key: ApiKey; kind: "rotate" | "revoke" } | null>(null);

  useEffect(() => { if (sp.get("new")) { setCreating(true); setSp({}, { replace: true }); } }, [sp, setSp]);

  const create = useAction<void, Created>(() => "/api/keys", INV, { body: () => ({ name, type, expires_at: days === "never" ? null : new Date(Date.now() + Number(days) * 864e5).toISOString(), grants: (profiles.data ?? []).filter((p) => type === "universal" || p.provider === type).map((p) => ({ profile_id: p.id, model_pattern: "*", routes: p.provider === "claude" ? ["models", "chat.completions"] : ["models", "responses", "chat.completions"], requests_per_minute: 60 })) }) });
  const act = useAction<{ id: string; kind: "rotate" | "revoke" }, Created>((v) => `/api/keys/${v.id}/${v.kind}`, INV);

  return (
    <Page title="API keys" subtitle="Keys for the public gateway. Only the prefix is stored for display." actions={<Btn variant="primary" onClick={() => setCreating(true)}><Plus className="h-3.5 w-3.5" />Create key</Btn>}>
      {q.isLoading ? <LoadingRows /> : q.isError ? <ErrorState error={q.error} retry={q.refetch} /> : !q.data!.length ? (
        <Empty title="No keys" hint="Create a key to call the gateway from your apps." action={<Btn variant="primary" onClick={() => setCreating(true)}>Create key</Btn>} />
      ) : (
        <Table head={["Name", "Key", "Type", "Grants", "Expires", "Last used", "Status", ""]}>
          {q.data!.map((k) => (
            <tr key={k.id} className={cn(k.status === "revoked" && "text-muted-foreground")}>
              <td className="px-4 py-3 font-medium">{k.name}</td>
              <td className="mono px-4 py-3 text-[12px]">{k.prefix}…</td>
              <td className="px-4 py-3"><Tag>{k.type}</Tag></td>
              <td className="px-4 py-3">{k.grant_count}</td>
              <td className="whitespace-nowrap px-4 py-3">{k.expires_at ? relTime(k.expires_at) : "never"}</td>
              <td className="whitespace-nowrap px-4 py-3">{relTime(k.last_used_at)}</td>
              <td className="px-4 py-3"><Dot state={k.status} /></td>
              <td className="px-4 py-3">
                {k.status === "active" && (
                  <div className="flex justify-end gap-1.5">
                    <Btn onClick={() => setConfirm({ key: k, kind: "rotate" })}>Rotate</Btn>
                    <Btn variant="danger" onClick={() => setConfirm({ key: k, kind: "revoke" })}>Revoke</Btn>
                  </div>
                )}
              </td>
            </tr>
          ))}
        </Table>
      )}

      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent className="max-w-md rounded-2xl">
          <DialogHeader><DialogTitle className="text-base">Create API key</DialogTitle></DialogHeader>
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); create.mutate(undefined, { onSuccess: (r) => { setCreating(false); setName(""); setRaw(r.key); } }); }}>
            <Field label="Name"><input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} maxLength={60} placeholder="Production gateway" required /></Field>
            <Field label="Type">
              <div className="flex gap-2">
                {(["universal", "claude", "codex"] as const).map((t) => (
                  <button type="button" key={t} onClick={() => setType(t)} className={cn("h-9 flex-1 rounded-lg border text-[13px] capitalize", type === t && "border-foreground bg-secondary font-medium")}>{t}</button>
                ))}
              </div>
            </Field>
            <Field label="Expires">
              <select className={inputCls} value={days} onChange={(e) => setDays(e.target.value)}>
                <option value="30">In 30 days</option><option value="90">In 90 days</option><option value="365">In 1 year</option><option value="never">Never</option>
              </select>
            </Field>
            <div className="flex justify-end gap-2">
              <Btn type="button" onClick={() => setCreating(false)}>Cancel</Btn>
              <Btn variant="primary" disabled={!name.trim() || create.isPending}>{create.isPending ? "Creating…" : "Create"}</Btn>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <Confirm
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={confirm?.kind === "rotate" ? `Rotate ${confirm.key.name}?` : `Revoke ${confirm?.key.name ?? ""}?`}
        body={confirm?.kind === "rotate" ? "The current key stops working immediately and a new one is issued." : "Apps using this key will be rejected. This can't be undone."}
        confirmLabel={confirm?.kind === "rotate" ? "Rotate" : "Revoke"}
        busy={act.isPending}
        onConfirm={() => confirm && act.mutate({ id: confirm.key.id, kind: confirm.kind }, {
          onSuccess: (r) => { if (confirm.kind === "rotate") setRaw(r.key); },
          onSettled: () => setConfirm(null),
        })}
      />
      <RevealOnce open={!!raw} onClose={() => setRaw(null)} title="Your new API key" value={raw ?? ""} note="Store it in a secret manager. Don't paste it into chats, URLs or client code." />
    </Page>
  );
}
