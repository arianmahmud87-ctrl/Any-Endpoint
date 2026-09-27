import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowUp, Check, ChevronDown, Square, X } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { cn } from "@/lib/utils";
import { api, errorMessage } from "@/lib/api/client";
import { useProfiles } from "@/lib/api/hooks";
import type { PlaygroundRun, Provider } from "@/lib/api/types";
import { useSession } from "@/auth/session";
import { CopyBtn, Dot } from "@/components/kit";
import { ProviderIcon } from "@/components/ProviderIcon";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";

const MAX_PROMPT = 8000;
const MAX_OUTPUT = 20000;

interface Turn { id: string; prompt: string; model: string; profile: string; provider: Provider; output?: string; error?: string; latency?: number; }

export default function Playground() {
  const profiles = useProfiles();
  const qc = useQueryClient();
  const { addRun } = useSession();
  const usable = useMemo(() => (profiles.data ?? []).filter((p) => p.status !== "disabled"), [profiles.data]);
  const [profileId, setProfileId] = useState<string>("");
  const [model, setModel] = useState<string>("");
  const [prompt, setPrompt] = useState<string>("");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [running, setRunning] = useState<boolean>(false);
  const ctrl = useRef<AbortController | null>(null);
  const end = useRef<HTMLDivElement>(null);

  const profile = usable.find((p) => p.id === profileId);
  useEffect(() => {
    if (!profile && usable[0]) { setProfileId(usable[0].id); setModel(usable[0].allowed_models[0] ?? ""); }
  }, [usable, profile]);
  useEffect(() => { end.current?.scrollIntoView({ behavior: "smooth" }); }, [turns]);

  const run = async () => {
    const text = prompt.trim();
    if (!text || !profile || running) return;
    const id = Math.random().toString(36).slice(2);
    setTurns((t) => [...t, { id, prompt: text, model, profile: profile.label, provider: profile.provider }]);
    setPrompt("");
    setRunning(true);
    addRun({ id, title: text.slice(0, 48) });
    ctrl.current = new AbortController();
    try {
      const r = await api<PlaygroundRun & { choices?: Array<{ message?: { content?: string } }> }>("/api/playground/runs", { method: "POST", body: { profile_id: profile.id, model, prompt: text }, signal: ctrl.current.signal });
      const output = r.output ?? r.choices?.[0]?.message?.content ?? JSON.stringify(r, null, 2);
      setTurns((t) => t.map((x) => (x.id === id ? { ...x, output: output.slice(0, MAX_OUTPUT), latency: r.latency_ms } : x)));
      qc.invalidateQueries({ queryKey: ["usage"] });
      qc.invalidateQueries({ queryKey: ["summary"] });
    } catch (e) {
      const aborted = e instanceof DOMException && e.name === "AbortError";
      setTurns((t) => t.map((x) => (x.id === id ? { ...x, error: aborted ? "Stopped." : errorMessage(e) } : x)));
    } finally {
      setRunning(false);
    }
  };

  const composer = (
    <div className="w-full rounded-[22px] border bg-background p-3 shadow-[0_2px_12px_-4px_rgba(0,0,0,.08)] transition focus-within:shadow-[0_4px_20px_-6px_rgba(0,0,0,.14)]">
      <textarea
        value={prompt}
        onChange={(e) => setPrompt(e.target.value.slice(0, MAX_PROMPT))}
        onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); run(); } }}
        placeholder={profile ? "Ask anything, or task an agent…" : "Add a provider profile to start"}
        disabled={!profile}
        rows={2}
        className="max-h-60 min-h-[52px] w-full resize-none bg-transparent px-1.5 text-[15px] outline-none placeholder:text-muted-foreground/70"
      />
      <div className="flex items-center justify-between gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger className="flex h-7 items-center gap-1.5 rounded-lg px-2 text-[12.5px] text-muted-foreground hover:bg-secondary disabled:opacity-40" disabled={!usable.length}>
            {profile ? <><ProviderIcon provider={profile.provider} size={16} /><Dot state={profile.worker_status} label="" />{profile.label}</> : "No profile"}
            <ChevronDown className="h-3 w-3" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-60 rounded-xl">
            <DropdownMenuLabel className="text-[11.5px] font-normal text-muted-foreground">Provider profile</DropdownMenuLabel>
            {usable.map((p) => (
              <DropdownMenuItem key={p.id} onClick={() => { setProfileId(p.id); setModel(p.allowed_models[0] ?? ""); }}>
                <ProviderIcon provider={p.provider} size={16} /><Dot state={p.worker_status} label="" /><span className="ml-1 flex-1">{p.label}</span>{p.id === profileId && <Check className="h-3.5 w-3.5" />}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild><Link to="/app/profiles">Manage profiles</Link></DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <div className="flex items-center gap-2">
          {prompt.length > MAX_PROMPT * 0.8 && <span className="text-[11.5px] text-muted-foreground">{prompt.length}/{MAX_PROMPT}</span>}
          <DropdownMenu>
            <DropdownMenuTrigger className="flex h-7 items-center gap-1 rounded-lg px-2 text-[12.5px] hover:bg-secondary" disabled={!profile}>
              <span className="font-medium">{model || "Model"}</span><ChevronDown className="h-3 w-3 text-muted-foreground" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="rounded-xl">
              {profile?.allowed_models.map((m) => (
                <DropdownMenuItem key={m} onClick={() => setModel(m)}><span className="flex-1">{m}</span>{m === model && <Check className="ml-4 h-3.5 w-3.5" />}</DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          {running ? (
            <button onClick={() => ctrl.current?.abort()} className="flex h-9 w-9 items-center justify-center rounded-full bg-foreground text-background transition active:scale-90" aria-label="Stop">
              <Square className="h-3 w-3" fill="currentColor" />
            </button>
          ) : (
            <button onClick={run} disabled={!prompt.trim() || !profile} className={cn("flex h-9 w-9 items-center justify-center rounded-full transition active:scale-90", prompt.trim() ? "bg-foreground text-background" : "bg-muted-foreground/20 text-background")} aria-label="Run">
              <ArrowUp className="h-4 w-4" strokeWidth={2.2} />
            </button>
          )}
        </div>
      </div>
    </div>
  );

  if (turns.length === 0)
    return (
      <div className="flex h-full flex-col items-center justify-center px-5 pb-16">
        <div className="fade-in w-full max-w-[768px]">
          {composer}
          <p className="mt-3 text-center text-[12px] text-muted-foreground">
            {profiles.isError ? errorMessage(profiles.error) : !profiles.isLoading && !usable.length ? <><Link to="/app/profiles?first_setup=true" className="underline underline-offset-2">Add a provider profile</Link> to begin.</> : "Prompts and responses aren't stored in this browser."}
          </p>
        </div>
      </div>
    );

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[768px] space-y-8 px-5 py-8">
          <div className="flex justify-end">
            <button onClick={() => setTurns([])} disabled={running} className="flex items-center gap-1 text-[12px] text-muted-foreground hover:text-foreground disabled:opacity-40"><X className="h-3 w-3" />Clear</button>
          </div>
          {turns.map((t) => (
            <div key={t.id} className="fade-in space-y-4">
              <div className="flex justify-end"><div className="max-w-[85%] whitespace-pre-wrap rounded-2xl bg-secondary px-4 py-2.5 text-[14px]">{t.prompt}</div></div>
              <div className="flex gap-3">
                <ProviderIcon provider={t.provider} size={26} className="mt-0.5" />
                <div className="min-w-0 flex-1">
              {t.output !== undefined ? (
                <div>
                  <div className="whitespace-pre-wrap break-words text-[14.5px] leading-relaxed">{t.output}</div>
                  <div className="mt-3 flex items-center gap-3 text-[12px] text-muted-foreground">
                    <span>{t.profile}</span><span>·</span><span className="mono">{t.model}</span><span>·</span><span>{(t.latency! / 1000).toFixed(2)}s</span>
                    <CopyBtn value={t.output} />
                  </div>
                </div>
              ) : t.error ? (
                <div role="alert" className="rounded-xl bg-destructive/[.07] px-4 py-3 text-[13px] text-destructive">{t.error}</div>
              ) : (
                <div className="flex gap-1 py-2">{[0, 1, 2].map((i) => <span key={i} className="h-1.5 w-1.5 animate-pulse rounded-full bg-foreground/40" style={{ animationDelay: `${i * 150}ms` }} />)}</div>
              )}
                </div>
              </div>
            </div>
          ))}
          <div ref={end} />
        </div>
      </div>
      <div className="mx-auto w-full max-w-[768px] px-5 pb-5">{composer}</div>
    </div>
  );
}
