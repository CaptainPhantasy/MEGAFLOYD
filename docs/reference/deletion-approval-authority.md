# Protected deletion approvals

Permanent-deletion approval is stored by the separately privileged
`megafloyd-approval-authority` service. The normal engine connects to
`/private/var/run/megafloyd-approval-authority.sock` on macOS or
`/run/megafloyd-approval-authority.sock` on Linux, and requires the server's
kernel identity to be root. The engine account must be different from the service
account. Workspace settings and environment variables cannot change this
endpoint or select an unprotected fallback.

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
constructor always uses the protected service. A separate integration fixture
runs the real broker and a pinned test client under different OS identities. Its
socket override exists only in the test binary. That fixture is isolated from
the production application, service endpoint, and approval records.

## Operating prerequisites

The canonical socket is `/private/var/run/megafloyd-approval-authority.sock` on
macOS and `/run/megafloyd-approval-authority.sock` on Linux. These canonical paths
avoid trusting a symlinked service directory. The broker runs as root, and its
record directory must already exist with root ownership and mode `0700`.

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

On macOS the pinned engine must have a valid hardened-runtime signature without
debugger or runtime-exception entitlements. On Linux it must be a static binary;
startup disables same-user debugger access before model tools run. Unsupported
platforms and unproven privilege or executable identity fail closed.

## Administrator deployment

Build the service separately from the normal application, review it, and place
it in an administrator-owned directory. The exact engine executable and every
containing directory must also be protected from the engine account. Compute
the engine pin after all signing or packaging changes. Never pin a test binary,
interpreter, or executable in a writable checkout for production.

The service accepts explicit options:

```text
megafloyd-approval-authority \
  --socket <canonical-platform-socket> \
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
