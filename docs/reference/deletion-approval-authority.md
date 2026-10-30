# Protected deletion approvals

On Linux, permanent-deletion approval is stored by the separately privileged
`megafloyd-approval-authority` service. The normal engine connects to
`/run/megafloyd-approval-authority.sock` and requires the server's kernel
identity to be root. The engine account must differ from the service account.
Workspace settings and environment variables cannot change this endpoint or
select an unprotected fallback.

The macOS approval transport is deliberately unavailable. Darwin stream peer
metadata does not authenticate each message's actual writer after descriptor
inheritance. Both the authority server and client refuse before creating a
listener, sending a request, or accepting an approval record. Permanent deletion
remains blocked on macOS; use quarantine. Harmless shell commands remain usable.

The broker authenticates the connecting process using kernel peer credentials
and the administrator's pinned engine executable path and SHA-256. Production
approval requires an engine executable and containing directory chain that the
engine account cannot replace or modify. A binary built into a writable checkout
is not a production trust anchor. Pin only reviewed release executables, never
test executables or general-purpose interpreters.

The service owns its private record directory under its own OS account. Each
write names the previously read revision; stale writes are rejected. The guard
preserves the original two human answers, target identities, pending launches,
partial completions, and exact remaining targets inside these protected records.
A restarted engine reads the same protected history. Workspace-local
`deletions/<session>/record.json` files remain untrusted historical files; they
are neither imported nor accepted as approval, regardless of signatures or a
`source: human` field. This change preserves existing files.

If the service is absent, inaccessible, or rejects the client identity, permanent
deletion is denied before asking for confirmations. The error names the
administrator-owned approval service. Harmless commands and quarantine moves
remain available. Recovery requires the administrator to configure the reviewed
service and pin the intended engine release; weakening file permissions or
copying local approval JSON does not restore authority.

Unit tests inject a test-only in-memory authority explicitly. The production
constructor always uses the protected service. The Linux integration fixture
runs the real broker and a pinned test client under different OS identities.
Its socket override exists only in the test binary. The fixture is isolated
from the production application, service endpoint, and approval records.

Earlier macOS process-restart smoke runs are historical functional evidence;
they did not verify actual message-writer identity and provide no final transport
security proof. They are superseded by explicit macOS refusal tests. Supported
protocol tests run on Linux and are explicitly skipped, not counted as passing,
on macOS.

## Operating prerequisites

The supported Linux socket is `/run/megafloyd-approval-authority.sock`. This
canonical path avoids trusting a symlinked service directory. The broker runs
as root, and its record directory must already exist with root ownership and
mode `0700`. There is no supported macOS approval-service configuration.

The engine must run under an unprivileged account. Root, administrative groups,
container-administrator groups, and accounts granted sudo commands are refused.
The engine rejects root, known administrative groups, and visible local sudo
grants before trusting a root socket. A password request or unavailable local
query remains unknown: it may only defer to the protected broker, never grant
approval. The genuine broker must establish complete absence of sudo grants
using its own root policy query at startup, before issuing a fresh challenge,
and again before every record read or write. The client accepts neither the
challenge nor record data without the broker's account-policy verification.
No engine account or sudo-policy change is needed merely to make a local
read-only policy query available.

An engine account that can become root cannot be isolated from a root-owned
approval store. Do not configure an administrative personal account as an
allowed engine identity or treat file permissions as protection from root.
This repair does not silently change existing accounts, sudo policy, or the
production application's service setup.

The supported Linux engine must be a static binary; startup disables same-user
debugger access before model tools run. Unsupported transports and unproven
privilege or executable identity fail closed.

## Administrator deployment

Build the service separately from the normal application, review it, and place
it in an administrator-owned directory. The exact engine executable and every
containing directory must also be protected from the engine account. Compute
the engine pin after all signing or packaging changes. Never pin a test binary,
interpreter, or executable in a writable checkout for production.

The service accepts explicit options:

```text
megafloyd-approval-authority \
  --socket /run/megafloyd-approval-authority.sock \
  --state-dir <existing-root-owned-0700-directory> \
  --allow-uid <unprivileged-engine-uid> \
  --allow-executable <absolute-protected-engine-path>=<sha256>
```

It has no command-line client that can manufacture approval records. Every
connection must pass kernel process, executable, runtime, account, and fresh
challenge checks. Linux also authenticates the sender of every message chunk.
Record namespaces include the authenticated engine UID, so allowed accounts
cannot select each other's records. Competing broker instances cannot share the
same state lock. A stale service-owned socket is retained in quarantine during
restart rather than permanently removed.

A successful isolated test or a repository commit does not install this service
in production. Deployment must satisfy these prerequisites before permanent
deletion becomes available. Ordinary harmless shell commands and quarantine
remain usable while the service is absent.


## Input-channel boundary

Permanent-deletion confirmations require the direct in-process TUI input path.
Backend/server workspaces use a remote-only question service: protected deletion
questions fail promptly before entering the pending UI queue. Native desktop,
browser, API, and client/server TUI question endpoints cannot authorize a new
permanent deletion. Ordinary questions and recoverable quarantine moves remain
available through those interfaces. A valid, previously approved unchanged
remainder can still use its existing protected authorization.

Direct local mode uses `MEGAFLOYD_CLIENT_SERVER=0`; both confirmation stages still
need answers and the protected authority must be available. Its in-process UI
is a trusted-input assumption. It does not authenticate a physical human or
prevent same-account terminal automation; this change must not be described as
providing that stronger guarantee. An authenticated native desktop confirmation
channel is not implemented. RPC input is never relabeled as trusted local input.

Even on a mixed service, the RPC answer entry point atomically refuses a pending
protected request and releases the blocked operation with an explicit error.
The engine-owned protection marker is excluded from JSON and cannot be supplied
or cleared by an answer payload.
