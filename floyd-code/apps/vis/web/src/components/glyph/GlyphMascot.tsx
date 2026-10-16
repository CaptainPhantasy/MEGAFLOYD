import { useEffect, useState } from 'react';
import { useGlyphMood } from '../../hooks/useGlyphMood';
import { GLYPH_COLS, GLYPH_ROWS, type GlyphMoodSpec } from '../../lib/glyph';

interface GlyphMascotProps {
  /** Rendered height in px; the glyph keeps its 2:1 canon aspect. */
  height?: number;
  className?: string;
}

/** The FLOYD glyph, drawn as one SVG rect per lit pixel cell and coloured by
 *  the live mood from the engine. Frames animate per the guide's timing. */
export function GlyphMascot({ height = 14, className }: GlyphMascotProps) {
  const { spec, reason } = useGlyphMood();
  const frame = useAnimatedFrame(spec);
  const rows = spec.frames[frame]!;
  const label = `FLOYD glyph — ${spec.mood}: ${spec.caption} (${reason})`;

  return (
    <svg
      width={height * 2}
      height={height}
      viewBox={`0 0 ${GLYPH_COLS} ${GLYPH_ROWS}`}
      role="img"
      aria-label={label}
      className={className}
      shapeRendering="crispEdges"
    >
      <title>{label}</title>
      {rows.flatMap((row, y) =>
        Array.from(row).map((cell, x) =>
          cell === '#' ? (
            <rect key={`${x}.${y}`} x={x} y={y} width={1} height={1} fill={spec.color} />
          ) : null,
        ),
      )}
    </svg>
  );
}

function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(
    () => window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  );
  useEffect(() => {
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = () => setReduced(query.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);
  return reduced;
}

function useAnimatedFrame(spec: GlyphMoodSpec): number {
  const reduced = usePrefersReducedMotion();
  const animate = spec.frames.length > 1 && !reduced;
  const [index, setIndex] = useState(0);
  useEffect(() => {
    if (!animate) {
      setIndex(0);
      return;
    }
    let cancelled = false;
    let timer = 0;
    let frame = 0;
    const show = () => {
      setIndex(frame);
      timer = window.setTimeout(() => {
        if (cancelled) return;
        frame = (frame + 1) % spec.frames.length;
        show();
      }, spec.frameMs[frame]);
    };
    show();
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [animate, spec]);
  return animate ? index : 0;
}
