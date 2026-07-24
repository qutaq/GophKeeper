package testutil

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
	"github.com/qutaq/gophkeeper/internal/server/auth"
	"github.com/qutaq/gophkeeper/internal/server/data"
	grpcserver "github.com/qutaq/gophkeeper/internal/server/grpc"
	syncsvc "github.com/qutaq/gophkeeper/internal/server/sync"
)

const bufSize = 1024 * 1024

// TestServer bundles an in-memory gRPC server and clients for integration tests.
type TestServer struct {
	Users    *MemUsers
	Refresh  *MemRefresh
	Items    *MemItems
	Auth     *auth.Service
	Data     *data.Service
	Sync     *syncsvc.Service
	Conn     *grpc.ClientConn
	AuthCli  gophkeeperv1.AuthServiceClient
	DataCli  gophkeeperv1.DataServiceClient
	SyncCli  gophkeeperv1.SyncServiceClient
	listener *bufconn.Listener
	server   *grpc.Server
}

// StartTestServer starts a bufconn gRPC server with in-memory storage.
func StartTestServer(t *testing.T) *TestServer {
	t.Helper()

	users := NewMemUsers()
	refresh := NewMemRefresh()
	items := NewMemItems()

	ring, err := auth.NewKeyRing(auth.SigningKey{ID: "v1", Key: bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokenManager(auth.TokenManagerConfig{
		Keys:       ring,
		Issuer:     "gophkeeper-test",
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	authSvc := auth.NewService(users, refresh, tokens)
	dataSvc := data.NewService(items)
	syncService := syncsvc.NewService(items)

	lis := bufconn.Listen(bufSize)
	server := grpcserver.New(grpcserver.ServerConfig{
		Auth:             authSvc,
		Data:             dataSvc,
		Sync:             syncService,
		EnableReflection: false,
	})
	go func() {
		_ = server.Serve(lis)
	}()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", //nolint:staticcheck // bufconn dialer
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		server.Stop()
		t.Fatal(err)
	}

	ts := &TestServer{
		Users:    users,
		Refresh:  refresh,
		Items:    items,
		Auth:     authSvc,
		Data:     dataSvc,
		Sync:     syncService,
		Conn:     conn,
		AuthCli:  gophkeeperv1.NewAuthServiceClient(conn),
		DataCli:  gophkeeperv1.NewDataServiceClient(conn),
		SyncCli:  gophkeeperv1.NewSyncServiceClient(conn),
		listener: lis,
		server:   server,
	}
	t.Cleanup(ts.Close)
	return ts
}

// Close stops the test server.
func (ts *TestServer) Close() {
	if ts == nil {
		return
	}
	if ts.Conn != nil {
		_ = ts.Conn.Close()
	}
	if ts.server != nil {
		ts.server.Stop()
	}
	if ts.listener != nil {
		_ = ts.listener.Close()
	}
}

// AuthCtx attaches a Bearer access token to the outgoing context.
func AuthCtx(ctx context.Context, accessToken string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+accessToken)
}
