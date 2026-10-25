# MegaFloyd PR review corrections

The six code-review findings were reproduced and repaired in the canonical SanDisk checkout. The amended package `megafloyd-20261024T235658Z` is installed, running and published from clean commit `67deb1ae9723e77d7cdc52b3ce23fb430a7af509`. Actual browser and desktop handoff checks passed afterward; the original 23-message session and DeepSeek/high settings are preserved.

| Reported issue | Correction |
|---|---|
| Personal runtime could listen remotely without an access boundary | Enforce loopback before account state/runtime initialization; remote browser access uses the company mesh SSH tunnel. |
| Native subagents missing from browser views | Discover task descendants through the native child-session endpoint and validate their ancestry for transcript and approval access. |
| Forks/branches missing from session lists | Preserve user conversations returned by the native root/branch list in both API versions. |
| Explicit reasoning ignored during a model change | Initialize the destination model, then apply and validate the explicit reasoning choice. |
| Goal configuration started a second synthetic turn | Defer goal dispatch when it accompanies a prompt; save controls before sending the actual prompt once. Standalone goal start/resume remains available. |
| Batch archive/restore URLs returned 404 | Match the shipped single-colon archive/restore/delete routes. |

Each behavior has a failed-before/passed-after check. The combined web check passed all 18 tests and the affected TypeScript check. The native goal regression uses a real SQLite session store and counted accepted turns; it passed without a model call. The server was rebundled with all 561 artwork files and the worker unchanged. No live credentials, model settings or conversation content were changed by these checks.

All six code-review threads received the fix commit and test evidence and are resolved. Security review completed with zero findings on the original implementation; the network-boundary amendment also received independent source review and direct checks. CI passed for the fix commit. No late findings were present in the final observed snapshot.
