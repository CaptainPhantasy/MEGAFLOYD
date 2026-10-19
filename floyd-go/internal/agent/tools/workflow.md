Manages structured workflows for complex multi-step operations with checkpointing and recovery.

<when_to_use>
Use this tool when:
- Implementing a new feature that requires multiple coordinated steps
- Fixing a bug that needs analysis, verification, and safe rollback
- Performing code migrations or refactoring with safety checks
- Running multi-step processes that may need user approval at certain stages
- You need to track progress through a complex task with checkpoint persistence
</when_to_use>

<when_not_to_use>
Skip this tool when:
- Performing a single, straightforward task
- Quick one-off operations that don't need tracking
- Simple file edits or reads
</when_not_to_use>

<actions>
- **definitions**: List all available workflow templates
- **start**: Begin a new workflow execution
- **status**: Check the status of a workflow (current or by checkpoint ID)
- **resume**: Continue a paused workflow
- **cancel**: Stop a running workflow
- **approve**: Approve or reject a workflow step awaiting user input
- **list**: Show all saved workflows (optionally filtered by workflow name)
</actions>

<predefined_workflows>
1. **feature_implementation**: Analyze → Plan → Approve → Execute → Verify
2. **bug_fix_verification**: Reproduce → Analyze → Plan → Approve → Execute → Verify
3. **code_review**: Identify Changes → Review Logic → Review Style → Review Security → Summarize
4. **migration**: Analyze → Plan → Approve → Execute → Verify → Cleanup
5. **refactor**: Analyze → Ensure Tests → Plan → Approve → Execute → Verify
</predefined_workflows>

<workflow_steps>
- **analyze**: Gather information and analyze the codebase
- **plan**: Create a detailed implementation plan
- **execute**: Apply changes to the codebase
- **verify**: Run tests and validations
- **approve**: User approval gate (pauses workflow until approved)
</workflow_steps>

<checkpointing>
Workflows are automatically checkpointed to `.floyd/workflows/` and can survive restarts:
- Each step execution is saved
- Paused workflows can be resumed later
- Failed workflows maintain their state for debugging
</checkpointing>

<approval_gates>
Workflows can include approval steps that pause execution:
- Use `approve` action with `approved: true` to continue
- Use `approve` action with `approved: false` to cancel
- Check status to see if approval is needed
</approval_gates>

<examples>
Start a feature implementation workflow:
```json
{
  "action": "start",
  "workflow_name": "feature_implementation",
  "context": {
    "feature": "Add user authentication",
    "files": "internal/auth/"
  }
}
```

Check status of current workflow:
```json
{
  "action": "status"
}
```

Approve a paused workflow:
```json
{
  "action": "approve",
  "checkpoint_id": "abc123",
  "approved": true
}
```

List all saved workflows:
```json
{
  "action": "list"
}
```

Get available workflow definitions:
```json
{
  "action": "definitions"
}
```
</examples>

<tips>
- Start with `definitions` to see available workflows
- Use `status` frequently to track progress
- Workflows with approval gates will pause automatically
- Failed workflows attempt automatic rollback
- Checkpoint IDs are returned when starting a workflow
</tips>
