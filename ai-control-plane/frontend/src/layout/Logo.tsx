/** Any Endpoint brand mark: pulse inside a ring, monochrome, scales crisply at any size. */
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg viewBox="0 0 400 400" width={size} height={size} fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" role="img" aria-label="Any Endpoint" className="shrink-0 text-foreground">
      <circle cx="200" cy="200" r="152" strokeWidth="24" />
      <path d="M106 215H160L190 130L230 276L256 215H300" strokeWidth="24" />
    </svg>
  );
}
