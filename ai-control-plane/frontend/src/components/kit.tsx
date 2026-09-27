import { useState, type ReactNode } from "react";
import { Check, Copy, AlertCircle, Lock, RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";
import { errorMessage } from "@/lib/api/client";
import { ApiError } from "@/lib/api/types";
import { Skeleton } from "@/components/ui/skeleton";

export function Page({ title, subtitle, actions, children }: { title: string; subtitle?: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <div className="fade-in mx-auto w-full max-w-5xl px-5 py-8 md:px-10 md:py-12">
      <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight">{title}</h1>
          {subtitle && <p className="mt-1 text-[13px] text-muted-foreground">{subtitle}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      {children}
    </div>
  );
}

export function Btn({ variant = "ghost", className, ...p }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "ghost" | "outline" | "danger" }) {
  return (
    <button
      {...p}
      className={cn(
        "inline-flex h-8 items-center gap-1.5 rounded-lg px-3 text-[13px] font-medium transition active:scale-[.97] disabled:pointer-events-none disabled:opacity-40",
        variant === "primary" && "bg-foreground text-background hover:bg-foreground/85",
        variant === "ghost" && "bg-secondary hover:bg-accent",
        variant === "outline" && "border bg-background hover:bg-secondary",
        variant === "danger" && "bg-destructive/10 text-destructive hover:bg-destructive/15",
        className,
      )}
    />
  );
}

/** Plain-text status label (no colored indicator). */
export function Dot({ state, label }: { state: string; label?: string }) {
  const text = label ?? state;
  if (!text) return null;
  return <span className="text-[13px] capitalize">{text}</span>;
}

export function Tag({ children }: { children: ReactNode }) {
  return <span className="rounded-md bg-secondary px-1.5 py-0.5 text-[11.5px] font-medium text-muted-foreground">{children}</span>;
}

export function CopyBtn({ value, label = "Copy" }: { value: string; label?: string }) {
  const [done, setDone] = useState<boolean>(false);
  return (
    <Btn
      variant="outline"
      onClick={async () => {
        await navigator.clipboard.writeText(value);
        setDone(true);
        setTimeout(() => setDone(false), 1400);
      }}
    >
      {done ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
      {done ? "Copied" : label}
    </Btn>
  );
}

export function Rows({ children }: { children: ReactNode }) {
  return <div className="overflow-hidden rounded-xl border">{children}</div>;
}

export function Table({ head, children }: { head: string[]; children: ReactNode }) {
  return (
    <div className="overflow-x-auto rounded-xl border">
      <table className="w-full text-left text-[13px]">
        <thead>
          <tr className="border-b bg-[hsl(var(--canvas))] text-[12px] text-muted-foreground">
            {head.map((h) => <th key={h} className="whitespace-nowrap px-4 py-2.5 font-medium">{h}</th>)}
          </tr>
        </thead>
        <tbody className="divide-y">{children}</tbody>
      </table>
    </div>
  );
}

export function LoadingRows({ n = 5 }: { n?: number }) {
  return (
    <div className="space-y-2">
      {Array.from({ length: n }, (_, i) => <Skeleton key={i} className="h-11 w-full rounded-lg" />)}
    </div>
  );
}

export function Empty({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center rounded-xl border border-dashed px-6 py-14 text-center">
      <p className="font-medium">{title}</p>
      {hint && <p className="mt-1 max-w-sm text-[13px] text-muted-foreground">{hint}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorState({ error, retry }: { error: unknown; retry?: () => void }) {
  const denied = error instanceof ApiError && error.status === 403;
  return (
    <div className="flex flex-col items-center rounded-xl border px-6 py-14 text-center">
      {denied ? <Lock className="h-5 w-5 text-muted-foreground" /> : <AlertCircle className="h-5 w-5 text-muted-foreground" />}
      <p className="mt-3 font-medium">{denied ? "Permission denied" : "Couldn't load this"}</p>
      <p className="mt-1 text-[13px] text-muted-foreground">{errorMessage(error)}</p>
      {retry && !denied && (
        <Btn className="mt-4" variant="outline" onClick={retry}><RefreshCw className="h-3.5 w-3.5" />Retry</Btn>
      )}
    </div>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[12px] font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

export const inputCls = "h-9 w-full rounded-lg border bg-background px-3 text-[13px] outline-none transition focus:border-foreground/30 focus:ring-2 focus:ring-foreground/5";
