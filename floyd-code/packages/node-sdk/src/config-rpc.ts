import { resolveConfigPath } from '@legacy-ai/agent-core-v2';
import { z } from 'zod';

import { parseConfigString } from '#/config/index';
import { ErrorCodes, FloydError } from '#/errors';

export type FloydConfigValidationPathSegment = string | number;

export interface FloydConfigValidationIssue {
  readonly path: readonly FloydConfigValidationPathSegment[];
  readonly message: string;
}

export interface ResolveFloydConfigPathInput {
  readonly homeDir?: string | undefined;
  readonly configPath?: string | undefined;
}

export interface ValidateFloydConfigTomlInput {
  readonly text: string;
  readonly filePath?: string | undefined;
}

export interface FloydConfigRpc {
  resolveConfigPath(input?: ResolveFloydConfigPathInput): Promise<string>;
  validateConfigToml(input: ValidateFloydConfigTomlInput): Promise<void>;
}

export class FloydConfigRpcClient implements FloydConfigRpc {
  async resolveConfigPath(input: ResolveFloydConfigPathInput = {}): Promise<string> {
    return resolveConfigPath(input);
  }

  async validateConfigToml(input: ValidateFloydConfigTomlInput): Promise<void> {
    try {
      parseConfigString(input.text, input.filePath);
    } catch (error) {
      const validationIssues = extractValidationIssues(error);
      if (validationIssues !== undefined) {
        throw toConfigValidationError(error, validationIssues);
      }
      throw error;
    }
  }
}

export function createFloydConfigRpc(): FloydConfigRpc {
  return new FloydConfigRpcClient();
}

function toConfigValidationError(
  error: unknown,
  validationIssues: readonly FloydConfigValidationIssue[],
): FloydError {
  const details =
    error instanceof FloydError && error.details !== undefined
      ? { ...error.details, validationIssues }
      : { validationIssues };

  if (error instanceof FloydError) {
    return new FloydError(error.code, error.message, { details });
  }

  const message = error instanceof Error ? error.message : String(error);
  return new FloydError(ErrorCodes.CONFIG_INVALID, message, { details });
}

function extractValidationIssues(error: unknown): readonly FloydConfigValidationIssue[] | undefined {
  const zodError = findZodError(error);
  if (zodError === undefined) return undefined;
  return zodError.issues.map((issue) => ({
    path: issue.path.map((segment) =>
      typeof segment === 'number' ? segment : String(segment),
    ),
    message: issue.message,
  }));
}

function findZodError(error: unknown): z.ZodError | undefined {
  if (error instanceof z.ZodError) return error;
  if (error instanceof Error && error.cause instanceof z.ZodError) return error.cause;
  return undefined;
}
