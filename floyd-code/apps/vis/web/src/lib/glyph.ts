// FLOYD glyph mascot data — the 2×6 canon glyph from the "FLOYD CLI GLYPH —
// Small Glyph. Massive Personality." guide, drawn as pixel matrices instead of
// a flat image so colours and animation frames stay data-driven.
//
// The pixel canvas is 12×6 fine cells: each canon cell is a 2×2 block, so
// canon row 1 (eyes) is fine rows 2-3 and canon row 2 (grin) is fine rows 4-5.
// Fine rows 0-1 are headroom for floats that rise above the glyph ("?" for
// THINKING, zZ for SLEEPY, sparks for BRAINSTORM). The side bars sit in
// columns 0 and 11 across the eye rows, exactly as in the guide's canon art.

export const GLYPH_COLS = 12;
export const GLYPH_ROWS = 6;

export const GLYPH_MOODS = [
  'DEFAULT',
  'HAPPY',
  'EXCITED',
  'FOCUSED',
  'THINKING',
  'ANALYZING',
  'SUSPICIOUS',
  'SUCCESS',
  'WARNING',
  'ERROR',
  'SLEEPY',
  'BUILDING',
  'DEPLOYING',
  'BRAINSTORM',
  'STEALTH',
  'BROKEN',
] as const;

export type GlyphMood = (typeof GLYPH_MOODS)[number];

/** One animation frame: GLYPH_ROWS strings of GLYPH_COLS chars, `#` = lit. */
export type GlyphFrame = readonly string[];

export interface GlyphMoodSpec {
  mood: GlyphMood;
  /** ANSI-256 colour code from the guide. */
  ansi: number;
  /** Hex approximation of the guide's colour, for the web. */
  color: string;
  caption: string;
  frames: readonly GlyphFrame[];
  /** Per-frame dwell in ms, aligned with `frames`. */
  frameMs: readonly number[];
}

const GRIN_TEETH_TOP = '.###.##.###.';
const GRIN_BOTTOM = '..########..';
const GRIN_STRAIGHT_TOP = '.##########.';
const BARS_ONLY = '#..........#';

const DEFAULT_OPEN: GlyphFrame = [
  '............',
  '............',
  '#..##..##..#',
  '#.#..##..#.#',
  GRIN_TEETH_TOP,
  GRIN_BOTTOM,
];

const DEFAULT_BLINK: GlyphFrame = [
  '............',
  '............',
  BARS_ONLY,
  '#..##..##..#',
  GRIN_TEETH_TOP,
  GRIN_BOTTOM,
];

const HAPPY_UP: GlyphFrame = [
  '............',
  '.###....###.',
  '#.#......#.#',
  BARS_ONLY,
  GRIN_TEETH_TOP,
  GRIN_BOTTOM,
];

const EXCITED_A: GlyphFrame = [
  '............',
  '...#....#...',
  '#.###..###.#',
  '...#....#...',
  GRIN_STRAIGHT_TOP,
  GRIN_BOTTOM,
];

const EXCITED_B: GlyphFrame = [
  '............',
  '..#.#..#.#..',
  '#..........#',
  '..#.#..#.#..',
  GRIN_STRAIGHT_TOP,
  GRIN_BOTTOM,
];

const FOCUSED_FRAME: GlyphFrame = [
  '............',
  '............',
  '#.###..###.#',
  BARS_ONLY,
  GRIN_STRAIGHT_TOP,
  GRIN_BOTTOM,
];

const THINKING_GRIN_TOP = '...#######..';
const THINKING_GRIN_BOTTOM = '..#########.';

const THINKING_A: GlyphFrame = [
  '.........##.',
  '..###....#..',
  '#....#.....#',
  '#...#....#.#',
  THINKING_GRIN_TOP,
  THINKING_GRIN_BOTTOM,
];

const THINKING_B: GlyphFrame = [
  '........##..',
  '..###.......',
  '#.#........#',
  '#.#......#.#',
  THINKING_GRIN_TOP,
  THINKING_GRIN_BOTTOM,
];

const THINKING_C: GlyphFrame = [
  '.........##.',
  '.........#..',
  BARS_ONLY,
  '#..###...#.#',
  THINKING_GRIN_TOP,
  THINKING_GRIN_BOTTOM,
];

const ANALYZING_FRAME: GlyphFrame = [
  '............',
  '............',
  '#.####.###.#',
  '#..##..#...#',
  '.##.##.##.#.',
  GRIN_BOTTOM,
];

const SUSPICIOUS_FRAME: GlyphFrame = [
  '............',
  '............',
  '#.###..###.#',
  '#...#..#...#',
  GRIN_STRAIGHT_TOP,
  '..##.##.##..',
];

const SUCCESS_FRAME: GlyphFrame = HAPPY_UP;

const WARNING_FRAME: GlyphFrame = [
  '............',
  '............',
  '#..##..##..#',
  BARS_ONLY,
  GRIN_STRAIGHT_TOP,
  '..#......#..',
];

const ERROR_FRAME: GlyphFrame = [
  '............',
  '..#.#..#.#..',
  '#..#....#..#',
  '#.#.#..#.#.#',
  GRIN_STRAIGHT_TOP,
  '..##.##.##..',
];

const SLEEPY_FRAME: GlyphFrame = [
  '..........##',
  '.........##.',
  BARS_ONLY,
  '#..##..##..#',
  GRIN_STRAIGHT_TOP,
  GRIN_BOTTOM,
];

const BUILDING_FRAME: GlyphFrame = [
  '..#.#..#.#..',
  '..###..###..',
  '#.#.#..#.#.#',
  '#.###..###.#',
  GRIN_TEETH_TOP,
  GRIN_BOTTOM,
];

const DEPLOY_A: GlyphFrame = [
  '............',
  '..#....#....',
  '#..#....#..#',
  '#.#....#...#',
  GRIN_STRAIGHT_TOP,
  GRIN_BOTTOM,
];

const DEPLOY_B: GlyphFrame = [
  '............',
  '............',
  BARS_ONLY,
  '#..##..##..#',
  '.####.......',
  '..####......',
];

const DEPLOY_C: GlyphFrame = [
  '............',
  '............',
  BARS_ONLY,
  '#..##..##..#',
  '.#######....',
  '..#######...',
];

const DEPLOY_D: GlyphFrame = [
  '............',
  '............',
  BARS_ONLY,
  '#..##..##..#',
  GRIN_STRAIGHT_TOP,
  GRIN_BOTTOM,
];

const BRAINSTORM_FRAME: GlyphFrame = [
  '..#......#..',
  '..###..###..',
  '#.#.#..#.#.#',
  '#.###..###.#',
  GRIN_STRAIGHT_TOP,
  '..##.##.##..',
];

const STEALTH_FRAME: GlyphFrame = [
  '............',
  '............',
  BARS_ONLY,
  '#...##..##.#',
  '..#########.',
  '............',
];

const BROKEN_FRAME: GlyphFrame = [
  '............',
  '..#......#..',
  '#..#....#..#',
  '#.#......#.#',
  '.####..####.',
  '..###..###..',
];

export const GLYPH_MOOD_SPECS: Record<GlyphMood, GlyphMoodSpec> = {
  DEFAULT: {
    mood: 'DEFAULT',
    ansi: 36,
    color: '#22d3ee',
    caption: 'Ready. Always.',
    frames: [DEFAULT_OPEN, DEFAULT_BLINK],
    frameMs: [2600, 180],
  },
  HAPPY: {
    mood: 'HAPPY',
    ansi: 32,
    color: '#4ade80',
    caption: 'Good stuff!',
    frames: [HAPPY_UP, DEFAULT_OPEN, HAPPY_UP],
    frameMs: [420, 420, 420],
  },
  EXCITED: {
    mood: 'EXCITED',
    ansi: 33,
    color: '#facc15',
    caption: "Let's go!",
    frames: [EXCITED_A, EXCITED_B, EXCITED_A],
    frameMs: [380, 380, 380],
  },
  FOCUSED: {
    mood: 'FOCUSED',
    ansi: 34,
    color: '#3b82f6',
    caption: 'Deep work...',
    frames: [FOCUSED_FRAME],
    frameMs: [1000],
  },
  THINKING: {
    mood: 'THINKING',
    ansi: 35,
    color: '#e879f9',
    caption: 'Processing...',
    frames: [THINKING_A, THINKING_B, THINKING_C],
    frameMs: [520, 520, 520],
  },
  ANALYZING: {
    mood: 'ANALYZING',
    ansi: 95,
    color: '#a78bfa',
    caption: 'Running the numbers.',
    frames: [ANALYZING_FRAME],
    frameMs: [1000],
  },
  SUSPICIOUS: {
    mood: 'SUSPICIOUS',
    ansi: 208,
    color: '#fb923c',
    caption: 'Hmmm...',
    frames: [SUSPICIOUS_FRAME],
    frameMs: [1000],
  },
  SUCCESS: {
    mood: 'SUCCESS',
    ansi: 92,
    color: '#5cf97c',
    caption: 'Nailed it.',
    frames: [SUCCESS_FRAME],
    frameMs: [1000],
  },
  WARNING: {
    mood: 'WARNING',
    ansi: 93,
    color: '#fde047',
    caption: 'Careful...',
    frames: [WARNING_FRAME],
    frameMs: [1000],
  },
  ERROR: {
    mood: 'ERROR',
    ansi: 91,
    color: '#ff5c5c',
    caption: 'Nope.',
    frames: [ERROR_FRAME],
    frameMs: [1000],
  },
  SLEEPY: {
    mood: 'SLEEPY',
    ansi: 90,
    color: '#94a3b8',
    caption: 'Later...',
    frames: [SLEEPY_FRAME],
    frameMs: [1000],
  },
  BUILDING: {
    mood: 'BUILDING',
    ansi: 96,
    color: '#38bdf8',
    caption: 'Compiling...',
    frames: [BUILDING_FRAME],
    frameMs: [1000],
  },
  DEPLOYING: {
    mood: 'DEPLOYING',
    ansi: 37,
    color: '#2dd4bf',
    caption: 'Shipping...',
    frames: [DEPLOY_A, DEPLOY_B, DEPLOY_C, DEPLOY_D],
    frameMs: [700, 420, 420, 420],
  },
  BRAINSTORM: {
    mood: 'BRAINSTORM',
    ansi: 213,
    color: '#f472d6',
    caption: 'Ideas incoming!',
    frames: [BRAINSTORM_FRAME],
    frameMs: [1000],
  },
  STEALTH: {
    mood: 'STEALTH',
    ansi: 90,
    color: '#6b7280',
    caption: 'Watching...',
    frames: [STEALTH_FRAME],
    frameMs: [1000],
  },
  BROKEN: {
    mood: 'BROKEN',
    ansi: 31,
    color: '#ef4444',
    caption: 'Fix me...',
    frames: [BROKEN_FRAME],
    frameMs: [1000],
  },
};
