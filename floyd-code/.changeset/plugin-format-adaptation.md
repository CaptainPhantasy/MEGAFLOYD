---
"@legacy-ai/floyd-code": minor
---

Floyd Code now installs Claude Code plugins and packs, Gemini CLI extensions, and bare skills folders by translating them into Floyd plugins; add `#name` to the source to install one entry from a pack. Unknown folders are swept for usable skills and commands instead of being refused, and every install reports what landed, what was skipped, and what looks suspect.
