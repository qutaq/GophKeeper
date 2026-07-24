// Package cli implements the GophKeeper cobra command tree.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qutaq/gophkeeper/internal/client/auth"
	"github.com/qutaq/gophkeeper/internal/client/crypto"
	"github.com/qutaq/gophkeeper/internal/client/data"
	"github.com/qutaq/gophkeeper/internal/client/storage"
	"github.com/qutaq/gophkeeper/internal/client/storage/sqlite"
	"github.com/qutaq/gophkeeper/internal/client/syncer"
	"github.com/qutaq/gophkeeper/internal/client/transport"
	"github.com/qutaq/gophkeeper/internal/config"
	"github.com/qutaq/gophkeeper/internal/model"
	"github.com/qutaq/gophkeeper/pkg/secure"
)

// Options are build-time CLI options.
type Options struct {
	Version   string
	BuildDate string
}

// NewRootCommand builds the root cobra command.
func NewRootCommand(opts Options) *cobra.Command {
	var (
		configPath string
		dataDir    string
		server     string
	)

	root := &cobra.Command{
		Use:           "gophkeeper-client",
		Short:         "GophKeeper CLI — private data vault with sync",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to config.yaml")
	root.PersistentFlags().StringVar(&dataDir, "data-dir", "", "local vault directory")
	root.PersistentFlags().StringVar(&server, "server", "", "gRPC server address host:port")

	flags := &globalFlags{configPath: &configPath, dataDir: &dataDir, server: &server}

	root.AddCommand(newVersionCmd(opts))
	root.AddCommand(newRegisterCmd(flags))
	root.AddCommand(newLoginCmd(flags))
	root.AddCommand(newAddCmd(flags))
	root.AddCommand(newListCmd(flags))
	root.AddCommand(newGetCmd(flags))
	root.AddCommand(newEditCmd(flags))
	root.AddCommand(newDeleteCmd(flags))
	root.AddCommand(newSyncCmd(flags))

	return root
}

type globalFlags struct {
	configPath *string
	dataDir    *string
	server     *string
}

type runtime struct {
	cfg   config.Client
	vault storage.Vault
	conn  *transport.Conn
	key   *crypto.MasterKey
	auth  *auth.Service
	data  *data.Service
	sync  *syncer.Service
}

func openRuntime(flags *globalFlags, needKey, needConn bool) (*runtime, error) {
	cfg, err := config.LoadClient(*flags.configPath)
	if err != nil {
		return nil, err
	}
	if *flags.dataDir != "" {
		cfg.DataDir = *flags.dataDir
	}
	if *flags.server != "" {
		cfg.ServerAddress = *flags.server
	}

	vault, err := sqlite.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	rt := &runtime{cfg: cfg, vault: vault}

	if needConn {
		conn, err := transport.Dial(cfg)
		if err != nil {
			_ = vault.Close()
			return nil, err
		}
		rt.conn = conn
		rt.auth = auth.NewService(vault, conn)
		rt.sync = syncer.NewService(vault, conn)
	} else {
		rt.auth = auth.NewService(vault, nil)
	}

	if needKey {
		pass, err := readPassword("Master password: ")
		if err != nil {
			rt.Close()
			return nil, err
		}
		key, err := rt.auth.Unlock(context.Background(), pass)
		if err != nil {
			rt.Close()
			return nil, fmt.Errorf("unlock vault: %w", err)
		}
		rt.key = key
		rt.data = data.NewService(vault, key)
	}
	return rt, nil
}

func (rt *runtime) Close() {
	if rt == nil {
		return
	}
	if rt.key != nil {
		rt.key.Zero()
		rt.key = nil
	}
	if rt.conn != nil {
		_ = rt.conn.Close()
	}
	if rt.vault != nil {
		_ = rt.vault.Close()
	}
}

func newVersionCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print client version and build date",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "gophkeeper-client %s (%s)\n", opts.Version, opts.BuildDate)
		},
	}
}

// readPassword reads a secret from the terminal (or stdin). Caller owns the
// returned slice and must zero it (DeriveKey / Unlock consume it).
func readPassword(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	if !term.IsTerminal(int(syscall.Stdin)) {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		return []byte(strings.TrimRight(line, "\r\n")), nil
	}
	raw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func readLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func wipe(b []byte) {
	secure.Zero(b)
}

func parseMeta(values []string) (model.Metadata, error) {
	out := model.Metadata{}
	for _, v := range values {
		k, val, ok := strings.Cut(v, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid meta %q, want key=value", v)
		}
		out[k] = val
	}
	return out, nil
}
