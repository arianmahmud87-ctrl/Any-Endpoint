import type { Provider } from "@/lib/api/types";
import { cn } from "@/lib/utils";

const RAYS = [0, 27, 58, 92, 118, 151, 183, 212, 243, 272, 301, 332];

function ClaudeMark() {
  return (
    <g stroke="#FBF1EC" strokeLinecap="round" fill="none">
      {RAYS.map((a, i) => (
        <line key={a} x1="50" y1="50" x2="50" y2={i % 2 ? 20 : 15} strokeWidth={i % 3 ? 7 : 8} transform={`rotate(${a} 50 50)`} />
      ))}
      <circle cx="50" cy="50" r="8" fill="#FBF1EC" stroke="none" />
    </g>
  );
}

function CodexMark() {
  return (
    <g fill="none" stroke="#fff" strokeWidth="5.5" strokeLinejoin="round">
      {[0, 60, 120, 180, 240, 300].map((a) => (
        <path key={a} d="M50 28 C60 20 76 24 78 38 L64 46 L50 38 Z" transform={`rotate(${a} 50 50)`} />
      ))}
    </g>
  );
}

/** Rounded-square brand icon for a provider. */
export function ProviderIcon({ provider, size = 18, className }: { provider: Provider; size?: number; className?: string }) {
  const claude = provider === "claude";
  return (
    <svg viewBox="0 0 100 100" width={size} height={size} role="img" aria-label={claude ? "Claude" : "Codex"} className={cn("shrink-0", className)}>
      <rect width="100" height="100" rx="24" fill={claude ? "#D97757" : "#10A37F"} />
      {claude ? <ClaudeMark /> : <CodexMark />}
    </svg>
  );
}

/** Provider icon plus its name, for tables and lists. */
export function ProviderBadge({ provider }: { provider: Provider }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-[12.5px] capitalize">
      <ProviderIcon provider={provider} size={16} />
      {provider}
    </span>
  );
}
