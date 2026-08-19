package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/qutaq/gophkeeper/internal/client/crypto"
	"github.com/qutaq/gophkeeper/internal/client/storage"
)

func newRegisterCmd(flags *globalFlags) *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Create a local vault and register on the server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if login == "" {
				var err error
				login, err = readLine("Login: ")
				if err != nil {
					return err
				}
			}
			accountPass, err := readPassword("Account password: ")
			if err != nil {
				return err
			}
			defer wipe(accountPass)

			masterPass, err := readPassword("Master password (encrypts local vault): ")
			if err != nil {
				return err
			}
			confirm, err := readPassword("Confirm master password: ")
			if err != nil {
				wipe(masterPass)
				return err
			}
			if string(masterPass) != string(confirm) {
				wipe(masterPass)
				wipe(confirm)
				return fmt.Errorf("master passwords do not match")
			}
			wipe(confirm)

			rt, err := openRuntime(flags, false, true)
			if err != nil {
				wipe(masterPass)
				return err
			}
			defer rt.Close()

			ctx := context.Background()
			key, err := rt.auth.InitVault(ctx, login, masterPass)
			if err != nil {
				return err
			}
			defer key.Zero()

			if err := rt.auth.Register(ctx, login, string(accountPass), key); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "registered and logged in")
			return nil
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "account login")
	return cmd
}

func newLoginCmd(flags *globalFlags) *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate on the server and store tokens locally",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if login == "" {
				var err error
				login, err = readLine("Login: ")
				if err != nil {
					return err
				}
			}
			accountPass, err := readPassword("Account password: ")
			if err != nil {
				return err
			}
			defer wipe(accountPass)

			masterPass, err := readPassword("Master password: ")
			if err != nil {
				return err
			}

			rt, err := openRuntime(flags, false, true)
			if err != nil {
				wipe(masterPass)
				return err
			}
			defer rt.Close()

			ctx := context.Background()
			var key *crypto.MasterKey
			_, metaErr := rt.vault.GetVaultMeta(ctx)
			switch {
			case errors.Is(metaErr, storage.ErrNotInitialized):
				key, err = rt.auth.InitVault(ctx, login, masterPass)
			case metaErr != nil:
				wipe(masterPass)
				return metaErr
			default:
				key, err = rt.auth.Unlock(ctx, masterPass)
				if err != nil {
					return fmt.Errorf("unlock vault: %w", err)
				}
			}
			if err != nil {
				return err
			}
			defer key.Zero()

			if err := rt.auth.Login(ctx, login, string(accountPass), "", key); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "logged in")
			return nil
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "account login")
	return cmd
}

func newSyncCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Bidirectional sync with the server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := openRuntime(flags, true, true)
			if err != nil {
				return err
			}
			defer rt.Close()

			ctx := context.Background()
			token, err := rt.auth.EnsureAccess(ctx, rt.key)
			if err != nil {
				return err
			}
			result, err := rt.sync.Sync(ctx, token)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sync ok: pulled=%d pushed=%d\n", result.Pulled, result.Pushed)
			return nil
		},
	}
}
