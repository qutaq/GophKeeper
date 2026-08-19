package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qutaq/gophkeeper/internal/client/data"
	"github.com/qutaq/gophkeeper/internal/model"
)

func newAddCmd(flags *globalFlags) *cobra.Command {
	var meta []string
	cmd := &cobra.Command{
		Use:   "add [credentials|text|binary|card|otp]",
		Short: "Add a private item to the local vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ, err := data.ParseType(args[0])
			if err != nil {
				return err
			}
			md, err := parseMeta(meta)
			if err != nil {
				return err
			}
			plain, err := collectPayload(typ)
			if err != nil {
				return err
			}

			rt, err := openRuntime(flags, true, false)
			if err != nil {
				return err
			}
			defer rt.Close()

			item, err := rt.data.Add(context.Background(), data.AddInput{
				Type:     typ,
				Plain:    plain,
				Metadata: md,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s (%s)\n", item.ID, item.Type.String())
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&meta, "meta", nil, "metadata key=value (repeatable)")
	return cmd
}

func newListCmd(flags *globalFlags) *cobra.Command {
	var (
		typeName string
		deleted  bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local vault items",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := openRuntime(flags, true, false)
			if err != nil {
				return err
			}
			defer rt.Close()

			var typ *model.DataType
			if typeName != "" {
				t, err := data.ParseType(typeName)
				if err != nil {
					return err
				}
				typ = &t
			}
			items, err := rt.data.List(context.Background(), typ, deleted)
			if err != nil {
				return err
			}
			for _, it := range items {
				dirty := ""
				if it.Dirty {
					dirty = " dirty"
				}
				del := ""
				if it.Deleted {
					del = " deleted"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tv%d%s%s\t%s\n",
					it.ID, it.Type.String(), it.Version, dirty, del, formatMeta(it.Metadata))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "filter by type")
	cmd.Flags().BoolVar(&deleted, "deleted", false, "include soft-deleted items")
	return cmd
}

func newGetCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a decrypted item and its metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := openRuntime(flags, true, false)
			if err != nil {
				return err
			}
			defer rt.Close()

			item, plain, err := rt.data.Get(context.Background(), args[0])
			if err != nil {
				return err
			}
			view, err := data.FormatPayload(item.Type, plain)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "id: %s\ntype: %s\nversion: %d\ndeleted: %v\ndirty: %v\nmetadata: %s\n\n%s\n",
				item.ID, item.Type.String(), item.Version, item.Deleted, item.Dirty, formatMeta(item.Metadata), view)
			return nil
		},
	}
}

func newEditCmd(flags *globalFlags) *cobra.Command {
	var meta []string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Replace plaintext/metadata of a local item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := openRuntime(flags, true, false)
			if err != nil {
				return err
			}
			defer rt.Close()

			item, _, err := rt.data.Get(context.Background(), args[0])
			if err != nil {
				return err
			}
			plain, err := collectPayload(item.Type)
			if err != nil {
				return err
			}
			var md model.Metadata
			if len(meta) > 0 {
				md, err = parseMeta(meta)
				if err != nil {
					return err
				}
			}
			updated, err := rt.data.Edit(context.Background(), args[0], plain, md)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", updated.ID)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&meta, "meta", nil, "replace metadata key=value (repeatable)")
	return cmd
}

func newDeleteCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a local item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := openRuntime(flags, true, false)
			if err != nil {
				return err
			}
			defer rt.Close()

			item, err := rt.data.SoftDelete(context.Background(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", item.ID)
			return nil
		},
	}
}

func collectPayload(typ model.DataType) ([]byte, error) {
	switch typ {
	case model.DataTypeCredentials:
		login, err := readLine("Item login: ")
		if err != nil {
			return nil, err
		}
		pass, err := readPassword("Item password: ")
		if err != nil {
			return nil, err
		}
		out, err := data.MarshalPayload(data.CredentialsPayload{Login: login, Password: string(pass)})
		wipe(pass)
		return out, err
	case model.DataTypeText:
		text, err := readLine("Text: ")
		if err != nil {
			return nil, err
		}
		return data.MarshalPayload(data.TextPayload{Text: text})
	case model.DataTypeBinary:
		path, err := readLine("File path: ")
		if err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out, err := data.MarshalPayload(data.DecodeBinary(path, raw))
		wipe(raw)
		return out, err
	case model.DataTypeCard:
		number, err := readLine("Card number: ")
		if err != nil {
			return nil, err
		}
		holder, err := readLine("Card holder: ")
		if err != nil {
			return nil, err
		}
		exp, err := readLine("Exp (MM/YY): ")
		if err != nil {
			return nil, err
		}
		cvv, err := readPassword("CVV: ")
		if err != nil {
			return nil, err
		}
		out, err := data.MarshalPayload(data.CardPayload{Number: number, Holder: holder, Exp: exp, CVV: string(cvv)})
		wipe(cvv)
		return out, err
	case model.DataTypeOTP:
		secret, err := readLine("OTP secret: ")
		if err != nil {
			return nil, err
		}
		issuer, err := readLine("Issuer: ")
		if err != nil {
			return nil, err
		}
		account, err := readLine("Account: ")
		if err != nil {
			return nil, err
		}
		return data.MarshalPayload(data.OTPPayload{Secret: secret, Issuer: issuer, Account: account})
	default:
		return nil, fmt.Errorf("unsupported type")
	}
}

func formatMeta(m model.Metadata) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}
