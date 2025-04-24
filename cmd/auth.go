package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

var (
	authWithToken bool

	authCmd = &cobra.Command{
		Use:   "auth <command> [flags]",
		Short: "Authorization commands.",
		Long:  `Authorization commands. These commands are used to authenticate with the Tempest API.`,
	}

	authShowCmd = &cobra.Command{
		Use:   "show",
		Short: "Show the current authentication token.",
		RunE:  authShowRunE,
	}

	authLoginCmd = &cobra.Command{
		Use:   "login [flags]",
		Short: "Login to the Tempest API.",
		Long: `Login to the Tempest API. This command will prompt for an API Key.
The key will be stored securely in the OS native keychain, or disk of the keychain is not available.`,
		RunE: authLoginRunE,
	}

	authLogoutCmd = &cobra.Command{
		Use:   "logout",
		Short: "Logout from the Tempest API.",
		Long:  `Logout from the Tempest API. This command will remove the API Key from the OS native keychain.`,
		RunE:  authLogoutRunE,
	}
)

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authShowCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)

	authLoginCmd.Flags().BoolVarP(&authWithToken, "with-token", "t", false, "Authenticate with a token passed on stdin.")
}

func authShowRunE(cmd *cobra.Command, args []string) error {
	token, err := tokenStore.Get()
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}

	cmd.Println(fmt.Sprintf(`TEMPEST_TOKEN="%s"`, os.Getenv("TEMPEST_TOKEN")))
	cmd.Println(fmt.Sprintf(`TEMPEST_TOKEN_FILE="%s"`, os.Getenv("TEMPEST_TOKEN_FILE")))
	cmd.Println(fmt.Sprintf("Token from keychain: %s", token))

	return nil
}

func authLoginRunE(cmd *cobra.Command, args []string) error {
	var input string
	var err error

	if authWithToken {
		input, err = readInputFromStdin(cmd)
		if err != nil {
			return err
		}

		if input == "" {
			return errors.New("empty token\n Try: tempest auth login --with-token < token.txt")
		}
	} else {
		if term.IsTerminal(int(syscall.Stdin)) {
			fmt.Print("Input your API Key: ")
			bytePassword, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return err
			}
			fmt.Println() // Print a newline after the password input

			input = strings.TrimSpace(string(bytePassword))

			if input == "" {
				return errors.New("API Key cannot be empty")
			}
		} else {
			input, err = readInputFromStdin(cmd)
			if err != nil {
				return err
			}

			if input == "" {
				return errors.New("empty token\n Try: tempest auth login < token.txt")
			}
		}
	}

	return tokenStore.Set(input)
}

func readInputFromStdin(cmd *cobra.Command) (string, error) {
	// Check if stdin is a terminal
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}

	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", errors.New("nothing read from stdin\n Try: tempest auth --with-token < token.txt")
	}

	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(b)), nil
}

func authLogoutRunE(cmd *cobra.Command, args []string) error {
	cmd.Println("Removing stored token from the keychain, if it exists.")
	return tokenStore.Delete()
}
