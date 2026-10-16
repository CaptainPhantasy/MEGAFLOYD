import { useState } from "react";
import {
  IconAlertTriangle,
  IconArrowLeft,
  IconCheck,
  IconCopy,
  IconFileSettings,
  IconFolderOpen,
  IconLoader2,
  IconRefresh,
  IconTerminal2,
} from "@tabler/icons-react";

import { bridge } from "@/services";
import { Button } from "@/components/ui/button";
import { FloydMascot } from "./FloydMascot";

interface Props {
  type: "loading" | "runtime-error" | "no-models" | "no-workspace" | "managed-provider-unconfigured";
  errorMessage?: string | null;
  onRefresh?: () => void;
  onBackToLogin?: () => void;
}

function ErrorDetails({ message }: { message?: string | null }) {
  const [copied, setCopied] = useState(false);

  if (!message) return null;

  const copyError = async () => {
    await navigator.clipboard.writeText(message);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2_000);
  };

  return (
    <div className="bg-muted/50 rounded-lg p-4 text-left space-y-2">
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <div className="flex items-center gap-2 min-w-0">
          <IconTerminal2 className="size-4" />
          <span>Error details</span>
        </div>
        <Button
          onClick={() => {
            void copyError();
          }}
          variant="ghost"
          size="xs"
          className="h-6 px-1.5 gap-1 shrink-0"
        >
          {copied ? <IconCheck className="size-3" /> : <IconCopy className="size-3" />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <pre className="max-h-36 overflow-auto whitespace-pre-wrap break-words text-xs bg-background rounded px-3 py-2 font-mono text-foreground">{message}</pre>
    </div>
  );
}

function SetupActions({ onRefresh, onBackToLogin }: Pick<Props, "onRefresh" | "onBackToLogin">) {
  return (
    <div className="flex flex-col min-[400px]:flex-row min-[400px]:justify-between gap-2 w-full">
      {onBackToLogin && (
        <Button onClick={onBackToLogin} variant="ghost" size="sm" className="gap-1 text-muted-foreground">
          <IconArrowLeft className="size-3" />
          Provider setup
        </Button>
      )}
      {onRefresh && (
        <Button onClick={onRefresh} variant="ghost" size="sm" className="gap-1 text-muted-foreground">
          <IconRefresh className="size-3" />
          Reload
        </Button>
      )}
    </div>
  );
}

function NoModelsContent({ onRefresh, onBackToLogin }: Pick<Props, "onRefresh" | "onBackToLogin">) {
  return (
    <>
      <div className="space-y-2">
        <div className="inline-flex items-center gap-2 text-amber-500">
          <IconAlertTriangle className="size-5" />
          <span className="text-sm font-medium">Model setup required</span>
        </div>
        <p className="text-xs text-muted-foreground">
          Configure a provider and model in your shared Floyd Code <code className="bg-muted px-1 rounded">config.toml</code> — no account or sign-in is required.
        </p>
      </div>

      <div className="bg-muted/50 rounded-lg p-4 text-left space-y-2">
        <div className="flex items-center gap-2 text-xs font-medium">
          <IconFileSettings className="size-4" />
          Shared Floyd Code configuration
        </div>
        <p className="text-xs text-muted-foreground">
          VS Code and the terminal UI use the same Floyd Code home, configuration, credentials, and sessions.
        </p>
      </div>

      <SetupActions onRefresh={onRefresh} onBackToLogin={onBackToLogin} />
    </>
  );
}

function ManagedProviderContent({ onRefresh, onBackToLogin }: Pick<Props, "onRefresh" | "onBackToLogin">) {
  return (
    <>
      <div className="space-y-2">
        <div className="inline-flex items-center gap-2 text-amber-500">
          <IconAlertTriangle className="size-5" />
          <span className="text-sm font-medium">Managed provider not configured</span>
        </div>
        <p className="text-xs text-muted-foreground">
          Your active model comes from the managed <code className="bg-muted px-1 rounded">managed:floyd-code</code>{" "}
          provider, which this build ships without a deployment: there is no sign-in to complete and no account to
          create.
        </p>
      </div>

      <div className="bg-muted/50 rounded-lg p-4 text-left space-y-2">
        <div className="flex items-center gap-2 text-xs font-medium">
          <IconFileSettings className="size-4" />
          Fix the configuration
        </div>
        <p className="text-xs text-muted-foreground">
          Set <code className="bg-muted px-1 rounded">default_model</code> to a model from your own provider in the
          shared Floyd Code <code className="bg-muted px-1 rounded">config.toml</code> — Provider setup has a working
          example — or point the managed provider at a deployment of your own by setting the{" "}
          <code className="bg-muted px-1 rounded">FLOYD_CODE_OAUTH_HOST</code> and{" "}
          <code className="bg-muted px-1 rounded">FLOYD_CODE_BASE_URL</code> environment variables. Then reload.
        </p>
      </div>

      <SetupActions onRefresh={onRefresh} onBackToLogin={onBackToLogin} />
    </>
  );
}

export function ConfigErrorScreen({ type, errorMessage, onRefresh, onBackToLogin }: Props) {
  if (type === "loading") {
    return (
      <div className="h-full flex items-center justify-center p-6">
        <div className="text-center space-y-4">
          <FloydMascot className="h-10 mx-auto opacity-50" />
          <div className="inline-flex items-center gap-2 text-muted-foreground">
            <IconLoader2 className="size-4 animate-spin" />
            <span className="text-sm">Starting Floyd Code…</span>
          </div>
        </div>
      </div>
    );
  }

  if (type === "no-workspace") {
    return (
      <div className="h-full flex items-center justify-center p-6">
        <div className="max-w-sm text-center space-y-6">
          <FloydMascot className="h-10 mx-auto opacity-50" />
          <div className="space-y-2">
            <div className="inline-flex items-center gap-2 text-amber-500">
              <IconFolderOpen className="size-5" />
              <span className="text-sm font-medium">No workspace open</span>
            </div>
            <p className="text-xs text-muted-foreground">Open a folder to start using Floyd Code.</p>
          </div>
          <Button
            onClick={() => {
              void bridge.openFolder();
            }}
            className="gap-2"
          >
            <IconFolderOpen className="size-4" />
            Open Folder
          </Button>
        </div>
      </div>
    );
  }

  if (type === "no-models") {
    return (
      <div className="h-full flex items-center justify-center p-6">
        <div className="max-w-sm text-center space-y-6">
          <FloydMascot className="h-10 mx-auto opacity-50" />
          <NoModelsContent onRefresh={onRefresh} onBackToLogin={onBackToLogin} />
        </div>
      </div>
    );
  }

  if (type === "managed-provider-unconfigured") {
    return (
      <div className="flex-1 min-h-0 overflow-y-auto p-6">
        <div className="max-w-sm mx-auto text-center space-y-6">
          <FloydMascot className="h-10 mx-auto opacity-50" />
          <ManagedProviderContent onRefresh={onRefresh} onBackToLogin={onBackToLogin} />
        </div>
      </div>
    );
  }

  return (
    <div className="flex-1 min-h-0 overflow-y-auto p-6">
      <div className="max-w-sm mx-auto text-center space-y-6">
        <FloydMascot className="h-10 mx-auto opacity-50" />
        <div className="space-y-2">
          <div className="inline-flex items-center gap-2 text-red-500">
            <IconAlertTriangle className="size-5" />
            <span className="text-sm font-medium">Floyd Code could not start</span>
          </div>
          <p className="text-xs text-muted-foreground">Check the error below. Full diagnostics are available in the Floyd Code output channel.</p>
        </div>
        <ErrorDetails message={errorMessage} />
        <div className="flex gap-2 justify-center">
          <Button
            onClick={() => {
              void bridge.showLogs();
            }}
            variant="outline"
            size="sm"
          >
            Show Logs
          </Button>
          {onRefresh && (
            <Button onClick={onRefresh} size="sm" className="gap-1">
              <IconRefresh className="size-3" />
              Retry
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
