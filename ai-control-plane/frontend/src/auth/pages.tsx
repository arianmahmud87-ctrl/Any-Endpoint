import { useEffect, type CSSProperties, type ReactNode } from "react";

const d = (ms: number): CSSProperties => ({ ["--d" as string]: `${ms}ms` });
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { KeyRound, Link2, Lock, Sparkles } from "lucide-react";
import { ProviderIcon } from "@/components/ProviderIcon";
import { useSession } from "./session";
import { Logo } from "@/layout/Logo";
import { Btn } from "@/components/kit";

const Center = ({ children }: { children: ReactNode }) => (
  <div className="flex min-h-full items-center justify-center bg-background p-6">{children}</div>
);

export function Spinner() {
  return <div className="h-5 w-5 animate-spin rounded-full border-2 border-foreground/15 border-t-foreground" />;
}

const Google = () => (
  <svg viewBox="0 0 24 24" className="h-4 w-4"><path fill="#4285F4" d="M22.5 12.3c0-.8-.1-1.5-.2-2.3H12v4.3h5.9a5 5 0 0 1-2.2 3.3v2.7h3.6c2-1.9 3.2-4.7 3.2-8Z"/><path fill="#34A853" d="M12 23c3 0 5.5-1 7.3-2.7l-3.6-2.7c-1 .7-2.2 1-3.7 1-2.9 0-5.3-1.9-6.2-4.5H2.1v2.8A11 11 0 0 0 12 23Z"/><path fill="#FBBC05" d="M5.8 14.1a6.6 6.6 0 0 1 0-4.2V7.1H2.1a11 11 0 0 0 0 9.8l3.7-2.8Z"/><path fill="#EA4335" d="M12 5.4c1.6 0 3.1.6 4.2 1.7l3.2-3.2A11 11 0 0 0 2.1 7.1l3.7 2.8C6.7 7.3 9.1 5.4 12 5.4Z"/></svg>
);

export function LoginPage() {
  const { status, login } = useSession();
  const err = new URLSearchParams(useLocation().search).get("error");
  if (status === "authed") return <Navigate to="/app/playground" replace />;
  const steps = [
    { icon: Link2, title: "Connect your subscription", text: "Link your ChatGPT or Claude plan in a few clicks." },
    { icon: KeyRound, title: "Get your API & endpoint", text: "Any Endpoint turns it into a ready-to-use API key and endpoint." },
    { icon: Sparkles, title: "Freedom welcomes you", text: "Use it in any app, tool or workflow you like." },
  ];
  const line1 = ["Use", "your", "Codex", "or", "Claude", "Code", "limit"];
  const line2 = ["anywhere", "you", "need."];
  return (
    <div className="grid min-h-full lg:grid-cols-[1.05fr_1fr]">
      <aside className="relative hidden overflow-hidden bg-[#f6f5f1] p-12 lg:flex lg:flex-col">
        <div className="pointer-events-none absolute inset-0 opacity-[.5] [background-image:radial-gradient(hsl(var(--foreground)/.09)_1px,transparent_1px)] [background-size:22px_22px] [mask-image:radial-gradient(ellipse_at_30%_40%,black,transparent_75%)] dot-drift" />
        <div className="reveal relative flex items-center gap-2.5 text-[15px] font-semibold tracking-tight" style={d(0)}><Logo size={28} />Any Endpoint</div>
        <div className="relative my-auto max-w-[440px]">
          <div className="reveal mb-6 flex items-center gap-2" style={d(80)}>
            <span className="float-a"><ProviderIcon provider="claude" size={28} /></span><span className="float-b"><ProviderIcon provider="codex" size={28} /></span>
            <span className="ml-1 text-[12.5px] text-muted-foreground">Claude Code · Codex</span>
          </div>
          <h2 className="text-[36px] font-semibold leading-[1.12] tracking-[-0.03em]">
            {line1.map((w, i) => <span key={w}><span className="word" style={d(150 + i * 70)}>{w}</span>{" "}</span>)}
            {line2.map((w, i) => <span key={w}><span className="word" style={d(150 + (line1.length + i) * 70)}><span className="shimmer-text">{w}</span></span>{i < line2.length - 1 ? " " : ""}</span>)}
          </h2>
          <span className="draw-line mt-6 block h-px w-16 bg-foreground/30" style={d(900)} />
          <p className="reveal mt-8 text-[11.5px] font-medium uppercase tracking-[.14em] text-muted-foreground" style={d(950)}>How it works</p>
          <ol className="relative mt-5 space-y-6">
            <span className="draw-line-y absolute left-[17.5px] top-9 bottom-9 w-px bg-border" style={d(1100)} />
            {steps.map(({ icon: I, title, text }, i) => (
              <li key={title} className="benefit reveal relative flex cursor-default gap-4" style={d(1050 + i * 150)}>
                <span className="benefit-icon relative flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border bg-background shadow-sm"><I className="h-4 w-4" strokeWidth={1.7} /></span>
                <div className="pt-0.5">
                  <p className="benefit-title text-[14px] font-medium"><span className="mono mr-2 text-[11.5px] text-muted-foreground">0{i + 1}</span>{title}</p>
                  <p className="mt-0.5 text-[13px] leading-relaxed text-muted-foreground">{text}</p>
                </div>
              </li>
            ))}
          </ol>
        </div>
        <p className="reveal relative text-[12px] text-muted-foreground" style={d(1500)}>© {new Date().getFullYear()} Any Endpoint</p>
      </aside>
      <main className="flex items-center justify-center bg-background p-6">
        <div className="fade-in flex w-full max-w-[380px] flex-col items-center text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl border bg-background shadow-[0_1px_2px_rgba(0,0,0,.04),0_8px_24px_-12px_rgba(0,0,0,.18)]"><Logo size={30} /></div>
          <h1 className="mt-7 text-[28px] font-semibold tracking-[-0.02em]">Sign in</h1>
          <p className="mt-2 max-w-[320px] text-[14px] leading-relaxed text-muted-foreground">Access your Any Endpoint workspace to manage subscriptions, keys and endpoints.</p>
          {err && <div role="alert" className="mt-6 w-full rounded-xl border border-destructive/20 bg-destructive/[.06] px-3.5 py-2.5 text-left text-[13px] text-destructive">We couldn't sign you in. Please try again or contact your administrator.</div>}
          <button onClick={() => login()} className="group mt-9 flex h-12 w-full items-center justify-center gap-3 rounded-full bg-foreground text-[14.5px] font-medium text-background shadow-[0_8px_20px_-10px_rgba(0,0,0,.5)] transition hover:opacity-90 active:scale-[.98]">
            <span className="flex h-6 w-6 items-center justify-center rounded-full bg-white"><Google /></span>
            Continue with Google
          </button>
          <div className="mt-6 w-full rounded-2xl border bg-secondary/40 p-4 text-left">
            <div className="flex items-center gap-2 text-[12.5px] font-medium">
              <span className="flex h-6 w-6 items-center justify-center rounded-lg border bg-background"><Lock className="h-3 w-3" strokeWidth={2} /></span>
              Protected access
            </div>
            <p className="mt-2 text-[12.5px] leading-relaxed text-muted-foreground">Access is restricted to authorized members of your organization. Sessions are encrypted and stored server-side. Your credentials never touch the browser.</p>
          </div>
          <p className="mt-8 max-w-[300px] text-[12px] leading-relaxed text-muted-foreground">By continuing, you agree to our <span className="font-medium text-foreground underline decoration-border underline-offset-4 hover:decoration-foreground">Terms of Service</span> and <span className="font-medium text-foreground underline decoration-border underline-offset-4 hover:decoration-foreground">Privacy Policy</span>.</p>
        </div>
      </main>
    </div>
  );
}

export function AuthCallback() {
  const { status } = useSession();
  const nav = useNavigate();
  useEffect(() => {
    if (status === "authed") nav("/app/playground", { replace: true });
    if (status === "anon") nav("/login?error=1", { replace: true });
  }, [status, nav]);
  return (
    <Center>
      <div className="flex flex-col items-center gap-3 text-[13px] text-muted-foreground"><Spinner />Signing you in…</div>
    </Center>
  );
}

export function AuthGuard({ children }: { children: ReactNode }) {
  const { status, expired, login, refetch } = useSession();
  if (status === "loading") return <Center><Spinner /></Center>;
  if (status === "anon") return <Navigate to="/login" replace />;
  if (status === "forbidden")
    return (
      <Center>
        <div className="text-center"><Lock className="mx-auto h-5 w-5 text-muted-foreground" /><p className="mt-3 font-medium">No access</p><p className="mt-1 text-[13px] text-muted-foreground">Your account isn't a member of any organization.</p></div>
      </Center>
    );
  if (status === "error")
    return (
      <Center>
        <div className="text-center"><p className="font-medium">Can't reach the control plane</p><Btn className="mt-4" variant="outline" onClick={() => refetch()}>Retry</Btn></div>
      </Center>
    );
  return (
    <>
      {children}
      {expired && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm">
          <div className="fade-in w-[320px] rounded-2xl border bg-background p-6 text-center shadow-xl">
            <p className="font-medium">Session expired</p>
            <p className="mt-1 text-[13px] text-muted-foreground">Sign in again to continue.</p>
            <Btn variant="primary" className="mt-5 w-full justify-center" onClick={() => login()}>Sign in</Btn>
          </div>
        </div>
      )}
    </>
  );
}
