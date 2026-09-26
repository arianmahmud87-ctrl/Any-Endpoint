export function relTime(iso: string | null): string {
  if (!iso) return "never";
  const d = (Date.now() - new Date(iso).getTime()) / 1000;
  const f = d < 0;
  const a = Math.abs(d);
  const s = a < 45 ? "just now" : a < 3600 ? `${Math.round(a / 60)}m` : a < 86400 ? `${Math.round(a / 3600)}h` : `${Math.round(a / 86400)}d`;
  if (s === "just now") return s;
  return f ? `in ${s}` : `${s} ago`;
}

export const num = (n: number): string => new Intl.NumberFormat("en").format(n);
