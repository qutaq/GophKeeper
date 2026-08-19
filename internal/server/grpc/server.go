package grpcserver

import (
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
)

// ServerConfig configures the gRPC server.
type ServerConfig struct {
	Auth             AuthService
	Data             DataService
	Sync             SyncService
	EnableReflection bool
	// TLSCreds enables TLS when non-nil. Required in production; nil allowed for in-process tests.
	TLSCreds credentials.TransportCredentials
}

// New builds a gRPC server with auth interceptor and registered services.
func New(cfg ServerConfig) *grpc.Server {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(AuthUnaryInterceptor(cfg.Auth)),
	}
	if cfg.TLSCreds != nil {
		opts = append(opts, grpc.Creds(cfg.TLSCreds))
	}
	s := grpc.NewServer(opts...)
	gophkeeperv1.RegisterAuthServiceServer(s, NewAuthServer(cfg.Auth))
	if cfg.Data != nil {
		gophkeeperv1.RegisterDataServiceServer(s, NewDataServer(cfg.Data))
	}
	if cfg.Sync != nil {
		gophkeeperv1.RegisterSyncServiceServer(s, NewSyncServer(cfg.Sync))
	}
	if cfg.EnableReflection {
		reflection.Register(s)
	}
	return s
}

// ListenAndServe starts serving on addr until the server is stopped.
func ListenAndServe(s *grpc.Server, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(lis)
}
