/** The pinned mark from the map, at text size. Inherits colour from `currentColor`. */
export function PinGlyph({ className, size = 12 }: { className?: string; size?: number }) {
  return (
    <svg width={size} height={size} viewBox="-6 -6 12 12" className={className} aria-hidden>
      <path d="M0,-5.5 Q1.1,-1.1 5.5,0 Q1.1,1.1 0,5.5 Q-1.1,1.1 -5.5,0 Q-1.1,-1.1 0,-5.5 Z" fill="currentColor" />
    </svg>
  );
}
