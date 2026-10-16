# README + GUI specification

Owner directive, 2026-10-16. This is a **deliverable spec**, not yet built. It exists so the
requirement survives to completion (standing instruction: hold on to memory).

## Deliverable 1 — `README.md` (written when the merge is complete)

- **Voice:** Floyd Labs brand voice — direct, confident, plain; no filler, no hedging.
- **Illustrations:** real screenshots of a **live user session** — not mockups — from:
  1. the TUI (terminal UI), and
  2. the web GUI.
- Placed where they show real work, with alt text describing what the screen shows.

## Deliverable 2 — GUI split into three independent verticals

**Directive:** the GUI is **three independent verticals**, *not* one interface with responsive
breakpoints. Each vertical is its own layout implementation; a viewport width does not share a
single stylesheet branch with the others.

| # | Vertical | Sizing rule (owner) |
|---|---|---|
| 1 | **Desktop** | Autosizes between *standard* and *widescreen*. |
| 2 | **Tablet** | Standard size = **iPad**. |
| 3 | **Mobile** | Standard size = **iPhone 16 Pro**. |

### Reference sizes — PROPOSED, confirm before build

These are proposals to pin the verticals to; they are **not verified device facts yet**.

| Vertical | Standard (portrait) | Wide / landscape | Notes |
|---|---|---|---|
| Desktop | 1280 × 800 | 1920 × 1080 | fluid between the two |
| Tablet (iPad) | 820 × 1180 (10.9″) | 1180 × 820 | *owner said "iPad … standard"* — modern iPad standard assumed |
| Mobile (iPhone 16 Pro) | 402 × 874 CSS px | — | 6.3″, 1206 × 2622 @3× |

### Open points to resolve before building

- **Which web GUI** — the committed console bundle (`apps/floyd-code/dist-web`, D-5), the
  hand-written company-platform SPA (`services/company-platform/platform.{html,js,css}`), and the
  `vis` GUI are three different web surfaces. The verticals must be applied to the intended one.
- **Tablet standard row** above (iPad generation/size).

## Gate (when this is worked)

- `README.md` exists in the Floyd Labs voice, with real TUI + web GUI screenshots.
- The three verticals render **independently** (separate implementations), verified at the three
  standard sizes above — not as one page at three breakpoints.
