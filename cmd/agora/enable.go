package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const environmentIdentityName = "environment"

func init() {
	rootCmd.AddCommand(newEnableCommand().cmd)
}

type enableCommand struct {
	description string
	host        string
	email       string
	cmd         *cobra.Command
}

func newEnableCommand() *enableCommand {
	cmd := &cobra.Command{
		Use:   "enable [account-token]",
		Short: "Enable a local agora environment",
		Long: "Enable a local agora environment.\n\n" +
			"Provide an account token directly, or omit it to authenticate with your account\n" +
			"email and password and obtain the token interactively.",
		Args: cobra.MaximumNArgs(1),
	}
	command := &enableCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.description, "description", "", "Optional environment description")
	cmd.Flags().StringVar(&command.host, "host", "", "Optional host value to report for this environment")
	cmd.Flags().StringVar(&command.email, "email", "", "Account email for interactive login when no token is provided")
	cmd.Run = command.run
	return command
}

func (cmd *enableCommand) run(_ *cobra.Command, args []string) {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}
	if root.IsEnabled() {
		panic("environment is already enabled; run 'agora disable' first")
	}

	apiEndpoint, _ := root.APIEndpoint()
	if apiEndpoint == "" {
		panic("api endpoint is not configured; set AGORA_API_ENDPOINT or run 'agora config set api_endpoint <url>'")
	}

	accountToken := ""
	if len(args) == 1 {
		accountToken = args[0]
	} else {
		accountToken, err = loginForAccountToken(apiEndpoint, cmd.email)
		if err != nil {
			panic(err)
		}
	}

	host := cmd.host
	if host == "" {
		host, err = os.Hostname()
		if err != nil {
			panic(err)
		}
	}

	description := cmd.description
	if description == "" {
		description = host
	}

	client := openAccountAPIClient(apiEndpoint, accountToken)
	req := &api.EnableEnvironmentRequest{}
	req.Description.SetTo(description)
	req.Host.SetTo(host)

	res, err := client.EnableEnvironment(context.Background(), req)
	if err != nil {
		panic(err)
	}

	enabled, ok := res.(*api.EnableEnvironmentResponse)
	if !ok {
		panic(fmt.Sprintf("enable environment failed: %T", res))
	}

	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: enabled.Environment.ID,
		AccountToken:  accountToken,
		ZitiIdentity:  enabled.Environment.ZitiIdentityId,
		APIEndpoint:   apiEndpoint,
	}); err != nil {
		panic(err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, enabled.EnrollmentJson); err != nil {
		_ = root.DeleteEnvironment()
		panic(err)
	}
	if err := reloadNetworkAgentIfRunning(root); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to reload agora network runtime: %v\n", err)
	}

	fmt.Printf("enabled environment '%s'\n", enabled.Environment.ID)
}

// loginForAccountToken prompts for any missing account credentials and
// exchanges them for the account's API token via the controller login
// endpoint. Prompts are written to stderr so stdout stays scriptable.
func loginForAccountToken(apiEndpoint, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		fmt.Fprint(os.Stderr, "Email: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read email: %w", err)
		}
		email = strings.TrimSpace(line)
	}
	if email == "" {
		return "", fmt.Errorf("email is required")
	}

	fmt.Fprint(os.Stderr, "Password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	password := string(passwordBytes)
	if password == "" {
		return "", fmt.Errorf("password is required")
	}

	// login is unauthenticated, so no account token is needed on this client.
	client := openAccountAPIClient(apiEndpoint, "")
	res, err := client.Login(context.Background(), &api.LoginRequest{Email: email, Password: password})
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	return accountTokenFromLoginResult(res)
}

// accountTokenFromLoginResult extracts the account token from a login response,
// mapping controller error variants to a CLI-friendly error.
func accountTokenFromLoginResult(res api.LoginRes) (string, error) {
	switch v := res.(type) {
	case *api.AccountTokenResponse:
		return v.AccountToken, nil
	case *api.LoginUnauthorized:
		return "", fmt.Errorf("login failed: invalid credentials")
	default:
		return "", fmt.Errorf("login failed: unexpected response %T", res)
	}
}
