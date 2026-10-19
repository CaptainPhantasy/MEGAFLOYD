package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/legacy-ai/floyd/internal/version"
	"github.com/spf13/cobra"
)

// authRuntime is the resolved configuration and token manager for one auth
// command invocation. Tests replace authRuntimeFactory.
type authRuntime struct {
	Manager   *megafloydauth.Manager
	Host      string
	StorePath string
}

// authRuntimeFactory builds the runtime from the environment. Tests replace
// it with a fake flow and a temporary store.
var authRuntimeFactory = buildAuthRuntime

func buildAuthRuntime() (*authRuntime, error) {
	cfg := megafloydauth.FromEnv(nil)
	if cfg.Version == "" {
		cfg.Version = version.Version
	}
	if cfg.Version == "" {
		cfg.Version = "devel"
	}
	manager, err := megafloydauth.NewManager(cfg)
	if err != nil {
		return nil, err
	}
	path, err := manager.CredentialPath()
	if err != nil {
		return nil, err
	}
	return &authRuntime{
		Manager:   manager,
		Host:      cfg.OAuthHost,
		StorePath: path,
	}, nil
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage the megafloyd device login",
	Long: `Manage the device-code login for the megafloyd inference server.

Tokens are stored under the megafloyd home (MEGAFLOYD_HOME, default
~/.megafloyd), never under the f7 CLI's ~/.floyd7 home.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in with a device code",
	Long: `Request a device code, print the approval URL, and store the token.

The token is written with mode 0600 under the megafloyd credentials
directory. Nothing is sent to a browser automatically.`,
	Args: cobra.NoArgs,
	RunE: runAuthLogin,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the stored device token",
	Args:  cobra.NoArgs,
	RunE:  runAuthLogout,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether a token is stored and when it expires",
	Long: `Report whether a token is stored and when it expires.

Access and refresh tokens are never printed.`,
	Args: cobra.NoArgs,
	RunE: runAuthStatus,
}

var authTokenCmd = &cobra.Command{
	Use:    "token",
	Short:  "Print the stored access token",
	Hidden: true,
	Long: `Print the stored access token to standard output.

This command exists for provider credential templates such as
$(floyd auth token). It prints a secret; do not run it where the output is
recorded.`,
	Args: cobra.NoArgs,
	RunE: runAuthToken,
}

func init() {
	authCmd.AddCommand(authLoginCmd, authLogoutCmd, authStatusCmd, authTokenCmd)
}

func runAuthLogin(cmd *cobra.Command, _ []string) error {
	runtime, err := authRuntimeFactory()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	fmt.Fprintf(out, "Requesting device authorization from %s\n", runtime.Host)
	token, err := runtime.Manager.Login(ctx, func(auth megafloydauth.DeviceAuthorization) {
		url := auth.VerificationURIComplete
		if url == "" {
			url = auth.VerificationURI
		}
		fmt.Fprintln(out, "\nOpen this URL to approve the login:")
		fmt.Fprintf(out, "\n  %s\n\n", url)
		if auth.UserCode != "" {
			fmt.Fprintf(out, "User code: %s\n\n", auth.UserCode)
		}
		if auth.ExpiresIn > 0 {
			fmt.Fprintf(out, "The code expires in %s.\n", auth.ExpiresIn.Round(time.Second))
		}
		fmt.Fprintln(out, "Waiting for approval...")
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("login cancelled")
		}
		return err
	}

	fmt.Fprintf(out, "\nLogged in. Token expires %s.\n", formatTokenExpiry(token.ExpiresAt))
	fmt.Fprintf(out, "Credentials: %s\n", runtime.StorePath)
	return nil
}

func runAuthLogout(cmd *cobra.Command, _ []string) error {
	runtime, err := authRuntimeFactory()
	if err != nil {
		return err
	}
	status, err := runtime.Manager.Status()
	if err != nil {
		return err
	}
	if err := runtime.Manager.Logout(); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if status.LoggedIn || status.Revoked {
		fmt.Fprintf(out, "Logged out. Removed %s\n", runtime.StorePath)
		return nil
	}
	fmt.Fprintf(out, "Not logged in. Nothing to remove at %s\n", runtime.StorePath)
	return nil
}

func runAuthStatus(cmd *cobra.Command, _ []string) error {
	runtime, err := authRuntimeFactory()
	if err != nil {
		return err
	}
	status, err := runtime.Manager.Status()
	if err != nil {
		return err
	}
	switch {
	case status.Revoked:
		return fmt.Errorf(
			"%w; run `%s`", megafloydauth.ErrLoginRevoked, authLoginHint(cmd),
		)
	case !status.LoggedIn:
		return fmt.Errorf(
			"%w; run `%s`", megafloydauth.ErrNotLoggedIn, authLoginHint(cmd),
		)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Logged in to %s.\n", runtime.Host)
	fmt.Fprintf(out, "  Credentials: %s\n", runtime.StorePath)
	if status.ExpiresAt.IsZero() {
		fmt.Fprintln(out, "  Expires: unknown")
	} else {
		fmt.Fprintf(
			out, "  Expires: %s (%s)\n",
			status.ExpiresAt.Format(time.RFC1123), formatRemaining(status.Remaining),
		)
	}
	fmt.Fprintf(out, "  Refresh due: %s\n", yesNo(status.RefreshDue))
	if status.Scope != "" {
		fmt.Fprintf(out, "  Scope: %s\n", status.Scope)
	}
	return nil
}

func runAuthToken(cmd *cobra.Command, _ []string) error {
	runtime, err := authRuntimeFactory()
	if err != nil {
		return err
	}
	token, err := runtime.Manager.AccessToken(cmd.Context())
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), token)
	return nil
}

func authLoginHint(cmd *cobra.Command) string {
	return cmd.Root().CommandPath() + " auth login"
}

func formatTokenExpiry(expiresAt int64) string {
	if expiresAt == 0 {
		return "at an unknown time"
	}
	return time.Unix(expiresAt, 0).Format(time.RFC1123)
}

func formatRemaining(remaining time.Duration) string {
	if remaining <= 0 {
		return "already expired"
	}
	return "in " + remaining.Round(time.Second).String()
}
