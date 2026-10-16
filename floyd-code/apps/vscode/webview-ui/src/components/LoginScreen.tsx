import { IconArrowRight, IconRefresh } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { FloydMascot } from "./FloydMascot";

interface LoginScreenProps {
  onLoginSuccess: () => void;
  onSkip: () => void;
}

const PROVIDER_CONFIG_EXAMPLE = `default_model = "my-gateway/gpt-4.1"

[providers.my-gateway]
type = "openai"
base_url = "https://your-gateway.example/v1"
api_key = "YOUR_API_KEY"

[models."my-gateway/gpt-4.1"]
provider = "my-gateway"
model = "gpt-4.1"`;

export function LoginScreen({ onLoginSuccess, onSkip }: LoginScreenProps) {
  return (
    <div className="h-full overflow-y-auto p-6">
      <div className="min-h-full flex items-center justify-center">
        <div className="max-w-sm w-full text-center space-y-6">
          <FloydMascot className="h-12 mx-auto" />
          <div className="space-y-2">
            <h1 className="text-lg font-semibold">Welcome to Floyd Code</h1>
            <div className="text-left space-y-2">
              <p className="text-xs leading-5">
                Floyd Code needs a model provider before it can chat, and no account or sign-in is
                required. VS Code and the terminal UI read the same shared Floyd Code{" "}
                <code className="bg-muted px-1 rounded">config.toml</code>.
              </p>
            </div>
          </div>

          <div className="bg-muted/50 rounded-lg p-3 text-left space-y-2">
            <p className="text-xs text-muted-foreground">
              Add a provider and a model to{" "}
              <code className="bg-muted px-1 rounded">~/.floyd-code/config.toml</code> — or{" "}
              <code className="bg-muted px-1 rounded">$FLOYD_CODE_HOME/config.toml</code> when{" "}
              <code className="bg-muted px-1 rounded">FLOYD_CODE_HOME</code> is set:
            </p>
            <pre className="max-h-52 overflow-auto text-xs leading-5 bg-background rounded px-3 py-2 font-mono text-foreground">
              {PROVIDER_CONFIG_EXAMPLE}
            </pre>
          </div>

          <div className="space-y-5">
            <div className="text-left space-y-1">
              <Button
                onClick={() => {
                  onLoginSuccess();
                }}
                className="w-full justify-center gap-2"
              >
                <IconRefresh className="size-4" />
                Reload
              </Button>
              <p className="text-[11px] text-muted-foreground leading-4">Save the file, then reload to pick it up.</p>
            </div>

            <div className="text-left space-y-1">
              <Button type="button" variant="outline" onClick={onSkip} className="w-full relative justify-center font-normal">
                <span>Continue</span>
                <IconArrowRight className="size-4 text-muted-foreground absolute right-3" />
              </Button>
              <p className="text-[11px] text-muted-foreground leading-4">Continue without a model configured.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
