package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
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
	tokenEnv := os.Getenv("TEMPEST_TOKEN")

	token, err := tokenStore.Get()
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}

	if token == "" {
		return errors.New("No token found. Please login with 'tempest auth login' or set the TEMPEST_TOKEN environment variable")
	}

	cmd.Println(fmt.Sprintf(`TEMPEST_TOKEN="%s"`, tokenEnv))
	cmd.Println(fmt.Sprintf("Token from keychain: %s", token))

	return nil
}

func authLoginRunE(cmd *cobra.Command, args []string) error {
	if authWithToken {
		// Check if stdin is a terminal
		stat, err := os.Stdin.Stat()
		if err != nil {
			return err
		}

		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return errors.New("Nothing read from stdin.\nTry: tempest auth --with-token < token.txt")
		}

		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}

		token := strings.TrimSpace(string(b))

		if token == "" {
			return errors.New("Empty token.\n Try: tempest auth --with-token < token.txt")
		}

		return tokenStore.Set(token)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Input your API Key: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			fmt.Println("API Key cannot be empty.")
			continue
		}

		return tokenStore.Set(input)
	}
}

func authLogoutRunE(cmd *cobra.Command, args []string) error {
	cmd.Println("Removing stored token from the keychain, if it exists.")
	return tokenStore.Delete()
}
