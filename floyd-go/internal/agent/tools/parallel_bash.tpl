Execute multiple bash commands in parallel for improved efficiency.

<cross_platform>
Uses mvdan/sh interpreter (Bash-compatible on all platforms including Windows).
Use forward slashes for paths: "ls C:/foo/bar" not "ls C:\foo\bar".
</cross_platform>

<when_to_use>
Use this tool when you need to run MULTIPLE independent commands that don't depend on each other's output.
Ideal for:
- Exploring multiple directories simultaneously
- Running multiple read-only queries (git status, file listings, etc.)
- Gathering system information from different sources
- Any situation where commands are independent and can run concurrently
</when_to_use>

<when_not_to_use>
- Commands that depend on each other (use sequential bash instead)
- Commands that modify shared state
- Single commands (use regular bash tool)
- Commands requiring specific execution order
</when_not_to_use>

<parameters>
- commands: Array of up to 4 commands, each with:
  - command: The bash command to execute
  - description: Brief description (max 30 chars)
- working_dir: Optional working directory for all commands
- timeout: Optional timeout in seconds (default 60, max 300)
</parameters>

<safety_rules>
- Maximum 4 commands per call
- Read-only commands (ls, git status, cat, etc.) execute without prompts
- Any write operations require user permission for ALL commands
- All commands share the same timeout
- If one command fails, others continue executing
</safety_rules>

<output_format>
Results are returned in command order with:
- Success/failure status for each command
- Individual command outputs
- Duration and exit codes
- Summary of overall success rate
</output_format>

<examples>
Good: 4 independent file listings
commands: [
  {"command": "ls -la src/", "description": "List source files"},
  {"command": "ls -la tests/", "description": "List test files"},
  {"command": "git status", "description": "Check git status"},
  {"command": "wc -l **/*.go", "description": "Count Go lines"}
]

Bad: Commands depending on each other
commands: [
  {"command": "cd src", "description": "Change to src"},
  {"command": "ls", "description": "List files"}  # This runs in parallel, not after cd!
]

Bad: Too many commands
commands: [ ... 5+ commands ... ]  # Max 4 allowed
</examples>

<performance_note>
Parallel execution typically provides 50-80% speedup when running multiple independent commands.
Best results when commands have similar execution times.
</performance_note>
