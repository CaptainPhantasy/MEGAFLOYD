package cmd

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/legacy-ai/floyd/internal/megafloydadmin"
	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/spf13/cobra"
)

// adminRuntime is the resolved client for one admin command invocation.
// Tests replace adminRuntimeFactory.
type adminRuntime struct {
	Client *megafloydadmin.Client
}

// adminRuntimeFactory builds the runtime from the environment. Tests
// replace it with an httptest-backed client.
var adminRuntimeFactory = buildAdminRuntime

func buildAdminRuntime() (*adminRuntime, error) {
	cfg := megafloydadmin.FromEnv(nil)
	mode, err := cfg.ResolvedAuthMode()
	if err != nil {
		return nil, err
	}
	if mode == megafloydadmin.AuthModeSession {
		client, err := megafloydadmin.New(cfg)
		if err != nil {
			return nil, err
		}
		return &adminRuntime{Client: client}, nil
	}

	auth, err := authRuntimeFactory()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = auth.Host
	}
	cfg.Tokens = auth.Manager
	client, err := megafloydadmin.New(cfg)
	if err != nil {
		return nil, err
	}
	return &adminRuntime{Client: client}, nil
}

func newAdminCommand() *cobra.Command {
	admin := &cobra.Command{
		Use:   "admin",
		Short: "Manage company accounts and access",
		Long: `Manage company accounts and access on the megafloyd admin plane.

The default session mode mirrors the admin web app: it signs in with an
administrator key, keeps the session cookie, and sends the CSRF token on
writes. Set MEGAFLOYD_ADMIN_KEY (or MEGAFLOYD_ADMIN_PASSWORD) to use it.
An unset MEGAFLOYD_ADMIN_AUTH_MODE selects session mode when a key is
present.

Bearer mode remains the fallback: it uses the stored megafloyd access
token from "megafloyd auth login". Force it with
MEGAFLOYD_ADMIN_AUTH_MODE=bearer.

Only routes observed on the live admin plane are used. Per-user tool and
model grants have no observed route and fail closed unless the operator
sets MEGAFLOYD_ADMIN_GRANT_PATH or MEGAFLOYD_ADMIN_REVOKE_PATH.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	admin.AddCommand(
		newAdminUsersCommand(),
		newAdminInviteCommand(),
		newAdminGrantCommand(),
		newAdminRevokeCommand(),
	)
	return admin
}

var adminCmd = newAdminCommand()

func newAdminUsersCommand() *cobra.Command {
	users := &cobra.Command{
		Use:   "users",
		Short: "Manage company accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	users.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List company accounts",
		Args:  cobra.NoArgs,
		RunE:  runAdminUsersList,
	})
	return users
}

func runAdminUsersList(cmd *cobra.Command, _ []string) error {
	runtime, err := adminRuntimeFactory()
	if err != nil {
		return err
	}
	users, err := runtime.Client.ListUsers(cmd.Context())
	if err != nil {
		return wrapAdminError(cmd, err)
	}

	out := cmd.OutOrStdout()
	if len(users) == 0 {
		fmt.Fprintln(out, "No accounts.")
		return nil
	}
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "USERNAME\tACCESS\tLAST SIGN-IN\tID")
	for _, user := range users {
		access := "disabled"
		if user.Enabled {
			access = "enabled"
		}
		fmt.Fprintf(
			writer, "%s\t%s\t%s\t%s\n",
			user.Username, access, user.LastLoginString(), user.ID,
		)
	}
	return writer.Flush()
}

func newAdminInviteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "invite <email>",
		Short: "Create a company invitation",
		Long: `Create a company invitation for a new account.

The admin API names this field "username", so the supplied value is sent
unchanged under that key. The server returns a private link that expires
in two days.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := adminRuntimeFactory()
			if err != nil {
				return err
			}
			invite, err := runtime.Client.Invite(cmd.Context(), args[0])
			if err != nil {
				return wrapAdminError(cmd, err)
			}
			if strings.TrimSpace(invite.URL) == "" {
				return fmt.Errorf(
					"%w: invite response did not include a url",
					megafloydadmin.ErrInvalidResponse,
				)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Invitation link: %s\n", invite.URL)
			return nil
		},
	}
}

func newAdminGrantCommand() *cobra.Command {
	var name string
	grant := &cobra.Command{
		Use:   "grant <email> <tool|model> [name]",
		Short: "Grant a tool or model to an account (unverified route)",
		Long: `Grant a tool or model to an account.

No per-user grant route is verified on the live admin plane. This command
fails with an "unverified endpoint" error unless the operator sets
MEGAFLOYD_ADMIN_GRANT_PATH (and optionally MEGAFLOYD_ADMIN_GRANT_METHOD).
The path may contain {email}, {kind}, and {name} placeholders.

Because no route is verified, the request body shape ({email, kind, name})
is also unverified.

The second argument may be a kind plus a colon-separated name
("tool:search"), or a kind with the name in the third argument.`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := parseAccessArgs(args, name)
			if err != nil {
				return err
			}
			runtime, err := adminRuntimeFactory()
			if err != nil {
				return err
			}
			result, err := runtime.Client.Grant(cmd.Context(), request)
			if err != nil {
				return wrapAdminError(cmd, err)
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"Grant request accepted (HTTP %d) for %s: %s %s\n",
				result.StatusCode, request.Email, request.Kind, request.Name,
			)
			return nil
		},
	}
	grant.Flags().StringVar(
		&name, "name", "",
		"Tool or model name when the second argument is only tool|model",
	)
	return grant
}

func newAdminRevokeCommand() *cobra.Command {
	var name string
	revoke := &cobra.Command{
		Use:   "revoke <email> [tool|model] [name]",
		Short: "Revoke account access or a tool/model grant",
		Long: `Revoke access for a company account.

With one argument, the account is disabled through the verified
POST /admin/api/users/{id}/disable route.

With a tool or model argument, this calls an unverified per-user revoke
route. It fails with an "unverified endpoint" error unless
MEGAFLOYD_ADMIN_REVOKE_PATH is set.`,
		Args: cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) > 1 {
				request, err := parseAccessArgs(args, name)
				if err != nil {
					return err
				}
				runtime, err := adminRuntimeFactory()
				if err != nil {
					return err
				}
				result, err := runtime.Client.RevokeGrant(cmd.Context(), request)
				if err != nil {
					return wrapAdminError(cmd, err)
				}
				fmt.Fprintf(
					out,
					"Revoke request accepted (HTTP %d) for %s: %s %s\n",
					result.StatusCode, request.Email, request.Kind, request.Name,
				)
				return nil
			}

			runtime, err := adminRuntimeFactory()
			if err != nil {
				return err
			}
			user, err := runtime.Client.DisableUser(cmd.Context(), args[0])
			if err != nil {
				return wrapAdminError(cmd, err)
			}
			fmt.Fprintf(
				out, "Disabled account %s (%s).\n",
				user.Username, user.ID,
			)
			return nil
		},
	}
	revoke.Flags().StringVar(
		&name, "name", "",
		"Tool or model name when the second argument is only tool|model",
	)
	return revoke
}

func parseAccessArgs(
	args []string, nameFlag string,
) (megafloydadmin.GrantRequest, error) {
	request := megafloydadmin.GrantRequest{
		Email: strings.TrimSpace(args[0]),
	}
	spec := strings.TrimSpace(args[1])
	switch {
	case len(args) == 3 && strings.TrimSpace(args[2]) != "":
		request.Kind = spec
		request.Name = strings.TrimSpace(args[2])
	case strings.Contains(spec, ":"):
		kind, name, _ := strings.Cut(spec, ":")
		request.Kind = strings.TrimSpace(kind)
		request.Name = strings.TrimSpace(name)
	default:
		request.Kind = spec
		request.Name = strings.TrimSpace(nameFlag)
	}
	request.Kind = strings.ToLower(request.Kind)

	if request.Email == "" {
		return request, errors.New("email is required")
	}
	if request.Kind != "tool" && request.Kind != "model" {
		return request, fmt.Errorf(
			"kind must be tool or model, got %q", request.Kind,
		)
	}
	if request.Name == "" {
		return request, fmt.Errorf(
			"name is required: use %s:<name> or --name", request.Kind,
		)
	}
	return request, nil
}

func wrapAdminError(cmd *cobra.Command, err error) error {
	switch {
	case errors.Is(err, megafloydauth.ErrNotLoggedIn),
		errors.Is(err, megafloydauth.ErrLoginRevoked):
		return fmt.Errorf(
			"%w; run `%s`", err, adminAuthHint(cmd),
		)
	case errors.Is(err, megafloydadmin.ErrUnauthorized):
		return fmt.Errorf(
			"%w; set MEGAFLOYD_ADMIN_KEY for session auth, or run `%s` "+
				"for Bearer auth",
			err, adminAuthHint(cmd),
		)
	default:
		return err
	}
}

func adminAuthHint(cmd *cobra.Command) string {
	return cmd.Root().CommandPath() + " auth login"
}
