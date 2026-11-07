# MegaFloyd package and whole-suite verification — 2026-10-31

Owner request: “test the last 36 packages, ensure the app still works with the whole megafloyd app suite”.

Result: all original 36 packages now have direct behavior coverage (76 named tests), and all 118 Go packages pass ordinary and race execution. The canonical local suite was tested through its actual Go CLI/TUI, shared server, packaged web engine, signed Electron desktop, and launcher.

## Verified results

| Check | Actual result | Evidence |
|---|---|---|
| Original 36 packages | 36/36 pass; 76 named tests; none skipped | `packages-36-tests.json`, full/race logs |
| Complete Go tests | Exit 0; 118 packages; 4120 passed test/subtest events; 22 skipped | `go-full.log`, `go-full-result.json` |
| Complete Go race tests | Exit 0; 118 packages; 4119 passed events; 23 skipped | `go-race.log`, `go-race-result.json` |
| Go static checks and build | `go vet ./...` and `go build ./...`, exit 0 | `go-vet-result.json`, `go-build-result.json` |
| Web/SDK monorepo | Exit 0; 16,778 passed; 54 skipped; zero failures | `node-final.json`, `node-final-exit.json` |
| Desktop module tests | 57 passed, zero failures/skips | `receipt-desktop-unit-action.json` |
| Actual packaged Go/web/CLI integration | 11 checks pass, same conversation retained across surfaces | `receipt-integration-final-action.json` |
| Actual desktop composer | 5 checks pass; typed prompt and reply rendered in shared session | `desktop-final-result.json`, `desktop-final.png` |
| Actual Go TUI | 22 terminal steps pass, including acknowledged question answer | `receipt-tui-final-action.json`, `tui/server-tui/screens/` |
| Instructions | Source fingerprint intact; generated copies and prompt pack synchronized; candidate embedded hash matches | `preservation-result.json`, `receipt-instructions-action.json` |
| Existing work and installation | 169 prior-work backups retained; 2,063 production bundle entries unchanged | `baseline.json`, `installed-baseline.json`, `preservation-result.json` |

Evidence filenames above are under `.tmp/suite-verification-20261031/` in this repository. Raw test exits were retained; skipped tests are not counted as passed. The complete skip lists are in the Go result JSON files and `node-skipped-tests.txt`. Go skips include existing cassette rerecording, fixture-helper entry points, platform-specific approval transports, optional Docker/clipboard/preview checks, intentionally disabled ripgrep paths in tests, and an existing skipped background-shell test; the race run also skips an existing model-config event test. None belongs to the requested 36-package gap.

## Repairs demonstrated by the new checks

| Area | Defect and correction |
|---|---|
| AI HTTP client | Invalid header name blocked requests; use a legal version-header name. Versioned API bases duplicated `/v1`; append resource paths while preserving encoded base paths and queries. |
| Background execution | Fix running-state race and KillAll deadlock; drain output before completion notification; terminate an owned command process group so ordinary shell children cannot outlive cancellation. Darwin behavior verified; Linux/Windows execution tests cross-compile, without claiming runtime verification there. |
| Workflow | Restore completed prerequisites on resume; fail blocked dependency graphs; preserve independent snapshots and atomic checkpoint writes; cancel active workers; retain ownership through rollback. The default executor now reports unsupported configured command/validation/rollback execution honestly rather than fabricating success. |
| Priority queue | Avoid reacquiring the queue mutex from heap length callbacks; test ordering, concurrency, purge, and empty behavior. |
| Pubsub | Serialize concurrent shutdown and release background-context subscriptions when the broker closes. |
| Telemetry | Correct SQLite TEXT/property/timestamp reads and error propagation; avoid mutating caller property maps. |
| Search/indexing | Rank all case-insensitive matches before limiting, retaining exact symbol matches with deterministic ties. |
| Dependency analysis and diff | Parse multiline Gemfiles; count changed source lines beginning `++` or `--` correctly. |
| Validation and versioning | Reject hostless HTTP(S) URLs; compare numeric semantic versions correctly. |
| Integer selection | Sign-extend the branchless mask before 64-bit selection, preserving negative and large values. |
| Node API snapshot | Existing bulk-plugin endpoint was missing from expected API routes. Proved auth, validation, and two-fixture install/list behavior over local HTTP, then added the single missing snapshot row. No production Node code changed. |

Focused failures before correction, exact commands, and source hashes are in `agent-a.md`, `agent-b.md`, `agent-c.md`, their manifests, and `node-api-repair.md`. Review additionally exposed shell-child cancellation and workflow rollback ownership; both received regression coverage and passed final race checks.

## All 36 packages

| Package | Passing named behavior tests |
|---|---|
| `floyd` | `TestMainEntryHelper`, `TestMainEntrypointHelpAndFailure` |
| `floyd/cmd/megafloyd-approval-authority` | `TestAuthorityEntryHelper`, `TestRepeatedIdentityFlagsRetainAllPins`, `TestAuthorityCLIRejectsInvalidIdentityBeforeStartup` |
| `floyd/internal/agent/agenttest` | `TestOfflineCoordinatorPersistsCancelledAcceptedTurn` |
| `floyd/internal/agent/hyper` | `TestFetchCreditsCacheAndHTTPResults` |
| `floyd/internal/agent/notify` | `TestRunCompleteEventPreservesCorrelationAndFailureState`, `TestNotificationWirePayloadRetainsAuthenticationContext` |
| `floyd/internal/ai` | `TestCompleteUsesValidHeadersAndVersionedEndpoint`, `TestCompleteStreamDeliversEventsAndReportsHTTPFailure`, `TestPromptTemplatesPreserveLiteralValuesAndDefaults`, `TestBuildURLPreservesVersionPrefixAndQuery` |
| `floyd/internal/ansiext` | `TestEscapeMakesControlSequencesVisible` |
| `floyd/internal/codebase` | `TestAnalyzeCodebaseAndSearchRespectIgnoredFiles`, `TestProjectDependencyManifests` |
| `floyd/internal/diff` | `TestGenerateDiffReportsActualLineChanges` |
| `floyd/internal/dns` | `TestResolverReportsCancellationAndExhaustion`, `TestResolverFallsBackToReachableNameserver` |
| `floyd/internal/environment` | `TestEnvironmentSummaryDistinguishesInstalledAndConfigured`, `TestMCPDiscoveryRequiresServerEntryPoint` |
| `floyd/internal/errors` | `TestSystemErrorDebugDetailsAreOptIn`, `TestUserErrorKeepsCauseAndRecoveryInstructions` |
| `floyd/internal/execution` | `TestBackgroundCancellationStopsShellAndChild`, `TestKillAllReturnsAndStopsBackgroundJobs`, `TestBackgroundCompletionIncludesAllOutput`, `TestExecuteRejectsDeniedCommandBeforeSideEffects`, `TestExecuteUsesWorkspaceEnvironmentAndExitStatus`, `TestBackgroundExitWaitsForOutputDelivery`, `TestForegroundTimeoutStopsShellChildren` |
| `floyd/internal/extensibility` | `TestSmartIndexPrefersYAMLAndHonorsDisabledEntries`, `TestSmartIndexRejectsInvalidYAMLAndFallsBackOnlyWhenMissing` |
| `floyd/internal/fileops` | `TestManagerWritesReadsListsAndDiffsWorkspaceFiles`, `TestManagerReportsMissingDirectoryAndReadLimit` |
| `floyd/internal/format` | `TestSpinnerCancellationKeysCancelWorkAndQuit` |
| `floyd/internal/frontendops` | `TestFrontendSessionExportImportRetainsMessagesAndIsIdempotent`, `TestFrontendOperationsValidateInputAndUseSelectedWorkspace` |
| `floyd/internal/fsops` | `TestTextFileLifecyclePreservesExistingFiles` |
| `floyd/internal/history` | `TestFileVersionsAndTurnRewindPreserveHistory` |
| `floyd/internal/hookruntime` | `TestDenyHookPreventsToolBodyAndRetainsCorrelation`, `TestNativeToolLifecycleRecordsProvenSuccessFailureAndSchemaRejection` |
| `floyd/internal/hookstore` | `TestMissingTraceDoesNotCreateDatabaseAndCorruptionIsReported`, `TestTracePaginationFiltersAndReadOnlyRestart` |
| `floyd/internal/intelligence` | `TestSymbolIndexFindsCaseInsensitiveExactMatchBeforePrefixes` |
| `floyd/internal/oauth` | `TestOAuthClientSurvivesTokenPersistenceAndExchangeFailuresRemainTyped`, `TestTokenExpiryRefreshWindowAndMissingExpiry` |
| `floyd/internal/oauth/hyper` | `TestDeviceFlowExchangeAndIntrospectionUseProviderContract`, `TestDeviceProviderErrorsAndCancellationAreNotSuccess` |
| `floyd/internal/paranoia` | `TestSelectPreservesWholeInteger`, `TestRuntimeArithmeticAndEnvironment` |
| `floyd/internal/plugins` | `TestPluginParsingValidationAndEscapedMetadata`, `TestDiscoverValidPluginsAndDisabledComponents` |
| `floyd/internal/pqueue` | `TestPriorityQueueOrderingDoesNotDeadlock`, `TestPriorityQueueConcurrentProducersAndPurge` |
| `floyd/internal/pubsub` | `TestBrokerConcurrentShutdown`, `TestBrokerFanoutBackpressureAndCancellation` |
| `floyd/internal/swagger` | `TestPublishedAPISchemaHasUsableClientContracts` |
| `floyd/internal/telemetry` | `TestTelemetryReportsTransportFailure`, `TestTelemetryOptOutAndHTTPDelivery`, `TestLocalStorePersistsQueriesAndTimestampStatistics` |
| `floyd/internal/ui/logo/example` | `TestLogoExampleRendersInTerminal` |
| `floyd/internal/ui/terminal` | `TestTerminalSessionExecutesInWorkspaceAndResizes`, `TestTerminalComponentRoutesFocusAndRendersOutput` |
| `floyd/internal/ui/util` | `TestStatusCommandsPreserveSeverityAndError`, `TestExecShellRejectsMalformedAndEmptyCommands` |
| `floyd/internal/validation` | `TestInputValidationBoundaries` |
| `floyd/internal/version` | `TestVersionComparisonUsesNumericComponents`, `TestBuildIdentityAndChangelogOutput` |
| `floyd/internal/workflow` | `TestDefaultExecutorDoesNotInventCommandSuccess`, `TestCheckpointRestartAndFiltering`, `TestBlockedWorkflowDoesNotClaimCompletion`, `TestWorkflowRestoresCompletedDependencies`, `TestWorkflowCancelStopsWorkerAndProtectsSnapshots`, `TestWorkflowApprovalResumeAfterRestart`, `TestWorkflowOwnsWorkerUntilRollbackFinishes`, `TestCheckpointRejectsTraversalAndRemainsReadableDuringSave` |

## Candidate and preservation

- Final test binary SHA-256: `132a442378df5b8c61b271c9ee6351448e84a9be852c5862a01b64a4018db6c8`.
- Signed isolated bundle: `.tmp/suite-verification-20261031/MEGAFLOYD.app`; launcher: `.tmp/suite-verification-20261031/bin/megafloyd`.
- Candidate version `2026.10.31`, build ID `suite-20261031`; engine, readable policy, desktop provenance, web release identity, and native signature checked.
- Git HEAD remains `245317e5dc78e60aed89fa8f4b4032a5207c8389` on `codex/megafloyd-modern-hooks`. Source changes are local and uncommitted; pre-existing work was preserved.
- Approved `MegaFloyd.md` SHA-256 remains `5bf0bb4a37990bfd1c7f7cc22c73e1ec2bfbfddac0d95952bae49652aa40945b`.
- No production installation, deployment, merge, force-push, or user-file deletion was performed. Tests use synthetic fixtures and isolated state. No live paid provider or shared company service was changed.
- The initial desktop copy inherited a broken Frameworks symlink from the installed base. The copied link was preserved in the test quarantine, and the final candidate passed complete signing/build verification. The original installed entry remains unchanged.
- An exploratory Node run used inherited provider settings and forced telemetry opt-out, which distorted unit expectations; it was stopped and replaced by clean-environment runs. The first clean run isolated the one stale API snapshot; the final complete run passes. A mistyped diagnostic policy flag was rejected and was not counted as policy verification; the supported `prompt policy --sha256` command passed.
- Application behavior claims are local integration results using deterministic fixture models. They do not assert production installation, live provider availability, or Linux/Windows desktop operation.
