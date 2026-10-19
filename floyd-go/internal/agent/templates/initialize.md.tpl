Analyze this codebase and create/update **{{.Config.Options.InitializeAs}}** as a project context file to help future agents work effectively in this repository.

**First**: Check if directory is empty or contains only config files. If so, stop and say "Directory appears empty or only contains config. Add source code first, then run this command to generate {{.Config.Options.InitializeAs}}."

**Goal**: Document what an agent needs to know to work in this codebase — commands, patterns, conventions, gotchas. This is a project context file, not an operating protocol. Behavioral rules are enforced by the harness at the system level.

**Discovery process**:

1. Check directory contents with `ls`
2. Identify project type from config files and directory structure
3. Find build/test/lint commands from config files, scripts, Makefiles, or CI configs
4. Read representative source files to understand code patterns
5. If {{.Config.Options.InitializeAs}} exists, read it and update the project-specific sections

**Content to include**:

- Essential commands (build, test, run, deploy, etc.) — whatever is relevant for this project
- Code organization and structure
- Naming conventions and style patterns
- Testing approach and patterns
- Important gotchas or non-obvious patterns
- Key dependencies and their purpose

**Format**: Use the following structure:

```markdown
# {{.Config.Options.InitializeAs}} — Project Context

## Commands
- Build: `...`
- Test: `...`
- Lint: `...`
- Run: `...`

## Structure
Brief description of directory layout and key files.

## Conventions
Naming, style, patterns observed in the code.

## Testing
How tests are organized, run, and what frameworks are used.

## Gotchas
Non-obvious patterns, known issues, things that break easily.
```

**Critical**: Only document what you actually observe. Never invent commands, patterns, or conventions. If you can't find something, don't include it.
