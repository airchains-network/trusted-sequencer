package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/ignite/cli/ignite/pkg/cosmosaccount"
	"github.com/spf13/cobra"
)

var CreateAccountCmd = &cobra.Command{
	Use:   "create-account [name]",
	Short: "Create a new Jucntion wallet",
	Long:  `Create a new Jucntion wallet with the specified name`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Error getting home directory", err)
			return
		}

		keysDir := filepath.Join(home, ".trusted-sequencer", "keys")
		if err := os.MkdirAll(keysDir, 0700); err != nil {
			fmt.Println("Error creating keys directory", err)
			return
		}

		fmt.Println("Initializing account registry",
			"name", name,
			"keys_dir", keysDir)

		// Initialize account registry
		registry, err := cosmosaccount.New(
			cosmosaccount.WithKeyringBackend(keyring.BackendFile),
			cosmosaccount.WithHome(keysDir),
		)
		if err != nil {
			fmt.Println("Error initializing account registry", err)
			return
		}

		// Create new account
		account, mnemonic, err := registry.Create(name)
		if err != nil {
			fmt.Println("Error creating account", err)
			return
		}

		// Get the address
		address, err := account.Address("air")
		if err != nil {
			fmt.Println("Error getting address", err)
			return
		}

		fmt.Printf("Account created successfully!\n")
		fmt.Printf("Name: %s\n", name)
		fmt.Printf("Address: %s\n", address)
		fmt.Printf("Mnemonic: %s\n", mnemonic)
		fmt.Println("\nIMPORTANT: Save your mnemonic phrase in a secure place!")
	},
}
