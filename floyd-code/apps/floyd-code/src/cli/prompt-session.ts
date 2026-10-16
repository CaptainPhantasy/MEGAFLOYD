import type {
  ConfigDiagnostics,
  CreateSessionOptions,
  FloydAuthFacade,
  FloydConfig,
  ListSessionsOptions,
  ResumeSessionInput,
  Session,
  SessionSummary,
  TelemetryProperties,
} from '@legacy-ai/floyd-code-sdk';

export interface PromptHarness {
  readonly homeDir: string;
  readonly auth: FloydAuthFacade;

  track(event: string, properties?: TelemetryProperties): void;

  ensureConfigFile(): Promise<void>;
  getConfig(): Promise<Pick<FloydConfig, 'defaultModel' | 'telemetry'>>;
  getConfigDiagnostics(): Promise<ConfigDiagnostics>;
  listSessions(options: ListSessionsOptions): Promise<readonly SessionSummary[]>;
  createSession(options: CreateSessionOptions): Promise<Session>;
  resumeSession(input: ResumeSessionInput): Promise<Session>;
  close(): Promise<void>;
}
