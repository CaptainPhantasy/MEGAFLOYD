# MCP Tools Reference Sheet
**Generated:** 2026-02-13
**Purpose:** Complete reference of all available MCP tools across all CLIs

## Quick Reference - Server Inventory

**18 MCP servers** with ~105+ tools total:

| Server | Tools | Purpose | Location |
|--------|-------|---------|----------|
| `lab-lead` | 6 tools | Lab inventory, tool discovery, agent spawning | `lab-lead-server/` |
| `floyd-runner` | 6 tools | Project detection, test/lint/build/format | `FLOYD_CLI/dist/mcp/runner-server.js` |
| `floyd-git` | 7 tools | Git status, diff, log, commit, branch | `FLOYD_CLI/dist/mcp/git-server.js` |
| `floyd-explorer` | 5 tools | Project map, symbols, file reading, scratchpad | `FLOYD_CLI/dist/mcp/explorer-server.js` |
| `floyd-patch` | 5 tools | Apply diffs, edit ranges, insert, delete | `FLOYD_CLI/dist/mcp/patch-server.js` |
| `floyd-devtools` | 6 tools | Type analysis, git bisect, dependency graphs | `floyd-devtools-server/` |
| `novel-concepts` | 10 tools | Budget allocation, context compression, reasoning | `novel-concepts-server/` |
| `floyd-safe-ops` | 3 tools | Impact simulation, safe operations | `floyd-safe-ops-server/` |
| `floyd-terminal` | 9 tools | Terminal commands, process management | `floyd-terminal-server/` |
| `gemini-tools` | 3 tools | Dependency visualization, bug freezing, trace replay | `gemini-tools-server/` |
| `pattern-crystallizer-v2` | 5 tools | Pattern extraction, MIT analysis | `pattern-crystallizer-v2/` |
| `context-singularity-v2` | 9 tools | Context packing, compression, orchestration | `context-singularity-v2/` |
| `hivemind-v2` | 11 tools | Multi-agent coordination, task distribution | `hivemind-v2/` |
| `omega-v2` | 6 tools | Meta-cognitive reasoning, advanced AI | `omega-v2/` |
| `novel-concepts` | 10 tools | AI-assisted concept generation | `novel-concepts-server/` |
| `4_5v_mcp` | 1 tool | Opus 4.5 vision/image analysis | ZAI API |
| `zai-mcp-server` | 8 tools | Image/video analysis, OCR, UI extraction, error diagnosis | ZAI API |
| `web-search-prime` | 1 tool | Web search with results | ZAI API |
| `web-reader` | 1 tool | Web page to markdown conversion | ZAI API |
| `zread` | 3 tools | GitHub repo analysis | ZAI API |

---

## Detailed Tool Reference

### lab-lead (6 tools) - NEW
**Path:** `/Volumes/Storage/MCP/lab-lead-server/dist/index.js`
**Purpose:** Central management server for the entire MCP Lab

| Tool | When to Call | Description |
|------|--------------|-------------|
| `lab_inventory` | Getting lab overview | Complete inventory of all servers, tools, capabilities |
| `lab_find_tool` | Finding right tool | Describe task, get recommended tools with server locations |
| `lab_get_server_info` | Server details | Get specific server info: location, tools, configuration |
| `lab_spawn_agent` | Spawning sub-agents | Generate config for specialist agents (coder, researcher, etc.) |
| `lab_sync_knowledge` | Updating knowledge | Sync embedded knowledge with actual lab state |
| `lab_get_tool_registry` | Getting tool list | Get compact tool registry for agent prompts |

**Agent Types for Spawning:**
- `general` - Core tools: supercache, runner, git, explorer
- `coder` - Full dev toolchain: + patch, devtools, terminal
- `researcher` - Web/GitHub: supercache, web-search-prime, web-reader, zread
- `architect` - Analysis: supercache, devtools, explorer
- `tester` - Testing: runner, git, terminal
- `full` - All local servers

---

### floyd-runner (6 tools)
**Path:** `/Volumes/Storage/FLOYD_CLI/dist/mcp/runner-server.js`

| Tool | When to Call | Strengths | Limitations |
|------|--------------|-----------|-------------|
| `detect_project` | Starting work in new directory | Auto-detects Node/Go/Rust/Python + package manager | Only detects 4 languages |
| `run_tests` | After code changes | Runs appropriate test command for project type | Requires `grantPermission=true` on first use |
| `format` | Before committing | Uses project's configured formatter | Requires `grantPermission=true` on first use |
| `lint` | Before committing | Uses project's configured linter | Requires `grantPermission=true` on first use |
| `build` | After changes | Uses project's configured build command | Requires `grantPermission=true` on first use |
| `check_permission` | Before running tests/build | Check if permission already granted | Read-only check |

**Permission System:** Runner caches permissions for 1 hour per project. Use `grantPermission=true` once per session to authorize.

---

### floyd-git (7 tools)
**Path:** `/Volumes/Storage/FLOYD_CLI/dist/mcp/git-server.js`

| Tool | When to Call | Strengths | Limitations |
|------|--------------|-----------|-------------|
| `git_status` | Before any git operation | Shows staged/unstaged files, branch, ahead/behind | Returns structured JSON |
| `git_diff` | Reviewing changes | Can diff specific files or all, supports staged | No word-level diff |
| `git_log` | Investigating history | Filter by author, file, date range | Max 100 commits by default |
| `git_commit` | Ready to commit | Warns for protected branches (main/master) | Requires message |
| `git_stage` | Before commit | Stage specific files or all | No interactive staging |
| `git_unstage` | Mistakenly staged | Unstage files or reset all | No interactive unstage |
| `git_branch` | Branch management | list/current/create/switch | No delete/rename |
| `is_protected_branch` | Before committing to main | Checks if branch is protected | Patterns: main, master, develop, production, release |

**Protected Branch Patterns:** main, master, development, develop, production, release/*

---

### floyd-explorer (5 tools)
**Path:** `/Volumes/Storage/FLOYD_CLI/dist/mcp/explorer-server.js`

| Tool | When to Call | Strengths | Limitations |
|------|--------------|-----------|-------------|
| `project_map` | Understanding codebase structure | Compressed directory tree, customizable depth | Ignore patterns limited to defaults |
| `read_file` | Reading file contents | Supports line ranges, chunking | No syntax highlighting |
| `list_symbols` | Navigating files | Extracts classes, functions, interfaces | Regex-based, not full AST |
| `smart_replace` | Surgical edits | Validates uniqueness before replacing | Throws on multiple matches |
| `manage_scratchpad` | Planning/workflow | Persistent scratchpad at `.floyd/scratchpad.md` | File-based, no versioning |

**Default Ignore Patterns:** node_modules, .git, dist, build, .floyd

---

### floyd-patch (5 tools)
**Path:** `/Volumes/Storage/FLOYD_CLI/dist/mcp/patch-server.js`

| Tool | When to Call | Strengths | Limitations |
|------|--------------|-----------|-------------|
| `apply_unified_diff` | Applying git patches | Parses unified diff format, safety checks | Requires exact diff format |
| `edit_range` | Editing specific lines | Replace line range with new content | Line-based only |
| `insert_at` | Adding new content | Insert at specific line number | Line-based only |
| `delete_range` | Removing code | Delete specific line range | Line-based only |
| `assess_patch_risk` | Before applying patch | Checks for binary/sensitive files, large hunks | Heuristic-based |

**Risk Assessment Levels:** low, medium, high based on:
- Binary file detection
- Sensitive file patterns (.env, credentials, locks)
- Large hunks (>50 lines)
- File deletions
- Multiple files affected

---

### floyd-devtools (6 tools)
**Path:** `/Volumes/Storage/MCP/floyd-devtools-server/dist/index.js`

| Tool | When to Call | Strengths | Limitations |
|------|--------------|-----------|-------------|
| `dependency_analyzer` | Detecting circular deps | Uses Tarjan's SCC algorithm | Single project only |
| `typescript_semantic_analyzer` | Debugging type errors | 4 actions: find_mismatches, trace_type, compare_types | Requires tsconfig.json |
| `monorepo_dependency_analyzer` | Monorepo blast radius | 4 actions: build_graph, analyze_blast, suggest_fix | Only for package.json-based projects |
| `build_error_correlator` | Cross-project build failures | Groups errors by code/message | TypeScript-focused parsing |
| `git_bisect` | Finding breaking commits | Binary search through commits | Requires good_commit + test_command |
| `benchmark_runner` | Performance testing | Statistical analysis of runs | Requires benchmark input |

**typescript_semantic_analyzer Actions:**
- `find_type_mismatches` - Find all TS2322 errors with type traces
- `trace_type` - Find all definitions and usages of a type
- `compare_types` - Compare two types, show onlyInA/onlyInB
- `suggest_type_fixes` - Suggest fixes for type errors

**monorepo_dependency_analyzer Actions:**
- `build_dependency_graph` - Full graph with nodes/edges/roots/leaves
- `analyze_blast_radius` - Directly and transitively affected packages
- `suggest_fix_order` - Topological sort for fixing broken packages
- `detect_config_issues` - Missing tsconfig.json detection

---

### novel-concepts (10 tools)
**Path:** `/Volumes/Storage/MCP/novel-concepts-server/dist/index.js`

| Tool | When to Call | Description |
|------|--------------|-------------|
| `compute_budget_allocator` | First call on every new session | Task complexity scoring → compute allocation |
| `adaptive_context_compressor` | Context window pressure | Semantic compression with importance scoring |
| `analogy_synthesizer` | Explaining complex concepts | Cross-domain analogy generation |
| `concept_web_weaver` | Knowledge graph operations | Build/query concept graphs |
| `consensus_protocol` | Multi-perspective decisions | Structured agreement finding |
| `distributed_task_board` | Complex task decomposition | Task breakdown and coordination |
| `episodic_memory_bank` | Session history | Store/retrieve episodic memories |
| `execution_trace_synthesizer` | Debugging workflows | Trace execution paths |
| `refactoring_orchestrator` | Code restructuring | Safe refactoring coordination |
| `semantic_diff_validator` | Change validation | Semantic diff analysis |

**Note:** Project memory is stored locally in `.floyd/.supercache` — do NOT use any external cache server.

**Storage Path:** `~/.floyd/supercache/{tier}/{key}.json`

---

### floyd-safe-ops (3 tools)
**Path:** `/Volumes/Storage/MCP/floyd-safe-ops-server/dist/index.js`

| Tool | When to Call | Description |
|------|--------------|-------------|
| `impact_simulate` | Before dangerous operations | Simulates impact of changes |
| `safe_operation` | Executing with safety | Run operations with rollback capability |
| `verify_operation` | After operation | Verify operation succeeded |

---

### floyd-terminal (9 tools)
**Path:** `/Volumes/Storage/MCP/floyd-terminal-server/dist/index.js`

| Tool | When to Call | Description |
|------|--------------|-------------|
| `start_process` | Spawn persistent process | Start a long-running process |
| `interact_with_process` | Send input to process | Send STDIN to running process |
| `list_processes` | List running processes | See all managed processes |
| `stop_process` | Stop a process | Terminate a running process |
| `get_process_output` | Get process output | Retrieve STDOUT/STDERR |
| `send_signal` | Send signal to process | Send Unix signals (SIGTERM, etc.) |
| `create_terminal` | Create interactive terminal | Spawn interactive shell |
| `execute_command` | Run one-off command | Execute and return output |
| `get_terminal_status` | Check terminal status | Get terminal state |

---

### gemini-tools (3 tools) - NEW
**Path:** `/Volumes/Storage/MCP/gemini-tools-server/dist/index.js`
**Purpose:** Three specialized tools for dependency visualization, bug freezing, and trace replay debugging

| Tool | When to Call | Description |
|------|--------------|-------------|
| `dependency_hologram` | Analyzing codebase coupling | Quantifies and visualizes hidden dependencies between files/modules |
| `failure_to_test_transmuter` | After runtime failures | Converts crashes and errors into permanent regression tests |
| `trace_replay_debugger` | Reproducing bugs | Creates standalone tests from execution traces |

**dependency_hologram** analyzes:
- Incoming and outgoing coupling between files
- Hidden dependencies via string literals and config keys
- Coupling weight (0-100%) based on dependency count
- Output formats: text (ASCII art), JSON, or DOT graph

**failure_to_test_transmuter** generates tests for:
- Frameworks: Jest, Vitest, Mocha, Pytest
- Auto-generates test code from crash context
- Preserves environment variables and input state
- Includes error expectations in generated tests

**trace_replay_debugger** creates:
- Standalone test files from execution traces
- Environment reconstruction from trace data
- Input/output state validation
- Error reproduction tests

---

### pattern-crystallizer-v2 (5 tools)
**Path:** `/Volumes/Storage/MCP/pattern-crystallizer-v2/dist/index.js`

| Tool | Description |
|------|-------------|
| `extract_pattern` | Extract reusable patterns from code |
| `crystallize_pattern` | Convert pattern to reusable form |
| `analyze_mit` | Analyze patterns using MIT methodology |
| `get_pattern` | Retrieve stored pattern |
| `list_patterns` | List all stored patterns |

---

### context-singularity-v2 (9 tools)
**Path:** `/Volumes/Storage/MCP/context-singularity-v2/dist/index.js`

| Tool | Description |
|------|-------------|
| `pack_context` | Pack context for efficient storage |
| `compress_context` | Compress context to save tokens |
| `unpack_context` | Unpack compressed context |
| `optimize_context` | Optimize context for LLM consumption |
| `orchestrate_context` | Orchestrate multiple contexts |
| `merge_contexts` | Merge multiple context packs |
| `analyze_context` | Analyze context characteristics |
| `cache_context` | Cache context for reuse |
| `validate_context` | Validate context integrity |

---

### hivemind-v2 (11 tools)
**Path:** `/Volumes/Storage/MCP/hivemind-v2/dist/index.js`

| Tool | Description |
|------|-------------|
| `register_agent` | Register a new agent |
| `submit_task` | Submit task to hivemind |
| `get_result` | Get task result |
| `list_agents` | List all agents |
| `coordinate_task` | Coordinate multi-agent task |
| `broadcast_message` | Send message to all agents |
| `agent_status` | Get agent status |
| `kill_agent` | Terminate an agent |
| `task_history` | Get task history |
| `agent_metrics` | Get agent performance metrics |
| `swarm_execute` | Execute swarm task |

---

### omega-v2 (6 tools)
**Path:** `/Volumes/Storage/MCP/omega-v2/dist/index.js`

| Tool | Description |
|------|-------------|
| `meta_reason` | Meta-cognitive reasoning |
| `self_improve` | Self-improvement routines |
| `analyze_pattern` | Deep pattern analysis |
| `synthesize` | Synthesize multiple inputs |
| `evaluate` | Evaluate options |
| `reflect` | Reflection on outcomes |

---

### novel-concepts (10 tools)
**Path:** `/Volumes/Storage/MCP/novel-concepts-server/dist/index.js`

| Tool | Description |
|------|-------------|
| `generate_concept` | Generate novel concepts |
| `explore_idea` | Explore an idea space |
| `combine_concepts` | Combine multiple concepts |
| `mutate_concept` | Mutate existing concept |
| `evaluate_concept` | Evaluate concept novelty |
| `find_analogies` | Find cross-domain analogies |
| `brainstorm` | Brainstorming session |
| `refine_concept` | Refine concept details |
| `validate_concept` | Validate concept feasibility |
| `concept_history` | Get concept generation history |

---

## External HTTP MCP Servers (ZAI API)

### 4_5v_mcp (1 tool)
**URL:** `https://api.z.ai/api/mcp/4_5v_mcp/mcp`

| Tool | When to Call | Description |
|------|--------------|-------------|
| `analyze_image` | Screenshot/diagram analysis | OCR, UI understanding, chart analysis |

### zai-mcp-server (8 tools)
**URL:** `https://api.z.ai/api/mcp/zai/mcp`

| Tool | Description |
|------|-------------|
| `ui_to_artifact` | Turn UI screenshots into code/prompts/specs |
| `extract_text_from_screenshot` | OCR screenshots for code, terminals, docs |
| `diagnose_error_screenshot` | Analyze error snapshots and propose fixes |
| `understand_technical_diagram` | Interpret architecture, flow, UML, ER diagrams |
| `analyze_data_visualization` | Read charts and dashboards for insights |
| `ui_diff_check` | Compare two UI shots for visual drift |
| `analyze_image` | General-purpose image understanding |
| `analyze_video` | Video analysis (MP4/MOV/M4V ≤8MB) |

### web-search-prime (1 tool)
**URL:** `https://api.z.ai/api/mcp/web_search_prime/mcp`

| Tool | Description |
|------|-------------|
| `webSearchPrime` | Web search with results |

### web-reader (1 tool)
**URL:** `https://api.z.ai/api/mcp/web_reader/mcp`

| Tool | Description |
|------|-------------|
| `webReader` | Fetch and convert web pages to markdown |

### zread (3 tools)
**URL:** `https://api.z.ai/api/mcp/zread/mcp`

| Tool | Description |
|------|-------------|
| `get_repo_structure` | Get GitHub repo directory tree |
| `read_file` | Read file from GitHub repo |
| `search_doc` | Search repo documentation |

---

## Configuration File Locations

| CLI | Config Path |
|-----|-------------|
| `claude` | `~/.claude/mcp.json` |
| `crush` | `~/.config/crush/crush.json` |
| `opencode` | `~/.config/gocodeo/User/globalStorage/.mcp/mcp.json` |
| `claude-code` | `~/.config/claude-code/mcp.json` |
| `cline` | `~/.cline/data/settings/cline_mcp_settings.json` |
| `gemini` | `~/.gemini/settings.json` |
| `qwen` | `~/.qwen/settings.json` |
| `grok` | `~/.grok/settings.json` |
| `codex` | `~/.codex/config.toml` |

**IDE Extensions (Cline):**
| IDE | Config Path |
|-----|-------------|
| Cursor | `~/Library/Application Support/Cursor/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| VSCode | `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |
| Trae | `~/Library/Application Support/Trae/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json` |

---

## Server Binary Paths

| Server | Path |
|--------|------|
| lab-lead | `/Volumes/Storage/MCP/lab-lead-server/dist/index.js` |
| floyd-patch | `/Volumes/Storage/FLOYD_CLI/dist/mcp/patch-server.js` |
| floyd-runner | `/Volumes/Storage/FLOYD_CLI/dist/mcp/runner-server.js` |
| floyd-git | `/Volumes/Storage/FLOYD_CLI/dist/mcp/git-server.js` |
| floyd-explorer | `/Volumes/Storage/FLOYD_CLI/dist/mcp/explorer-server.js` |
| floyd-devtools | `/Volumes/Storage/MCP/floyd-devtools-server/dist/index.js` |
| novel-concepts | `/Volumes/Storage/MCP/novel-concepts-server/dist/index.js` |
| floyd-safe-ops | `/Volumes/Storage/MCP/floyd-safe-ops-server/dist/index.js` |
| floyd-terminal | `/Volumes/Storage/MCP/floyd-terminal-server/dist/index.js` |
| pattern-crystallizer-v2 | `/Volumes/Storage/MCP/pattern-crystallizer-v2/dist/index.js` |
| context-singularity-v2 | `/Volumes/Storage/MCP/context-singularity-v2/dist/index.js` |
| hivemind-v2 | `/Volumes/Storage/MCP/hivemind-v2/dist/index.js` |
| omega-v2 | `/Volumes/Storage/MCP/omega-v2/dist/index.js` |
| novel-concepts | `/Volumes/Storage/MCP/novel-concepts-server/dist/index.js` |

---

## Best Practices

### Tool Selection Guide
```
Need to understand lab capabilities?       → lab-lead/lab_inventory
Need to find right tool for task?          → lab-lead/lab_find_tool
Need to spawn specialist agent?            → lab-lead/lab_spawn_agent
Need to understand project structure?      → floyd-explorer/project_map
Need to edit specific lines?               → floyd-patch/edit_range
Need to apply git patch?                   → floyd-patch/apply_unified_diff
Need to run tests?                         → floyd-runner/run_tests
Need type error diagnosis?                 → floyd-devtools/typescript_semantic_analyzer
Need to find breaking commit?              → floyd-devtools/git_bisect
Need to check git status?                  → floyd-git/git_status
Need to commit changes?                    → floyd-git/git_commit
Need to save reasoning for later?          → Write to .floyd/.supercache (local project file)
Need to analyze screenshot?                → 4_5v_mcp/analyze_image
```

### Debugging Workflow
```
1. lab_inventory → Understand available tools
2. detect_project → Understand project type
3. git_status → Check current state
4. typescript_semantic_analyzer → Find type errors
5. git_bisect → Find when error was introduced
6. monorepo_dependency_analyzer → Check blast radius
7. Write findings to .floyd/.supercache
```

---

*Last Updated: 2026-02-13*
*All configurations verified across 9 CLI tools + 3 IDE extensions*