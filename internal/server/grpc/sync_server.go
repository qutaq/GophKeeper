package grpcserver

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
	syncsvc "github.com/qutaq/gophkeeper/internal/server/sync"
)

// SyncService is the sync pull use-case required by gRPC handlers.
type SyncService interface {
	Pull(ctx context.Context, ownerID string, sinceVersion int64, sinceTimestamp time.Time) (*syncsvc.Result, error)
}

// SyncServer implements gophkeeper.v1.SyncService.
type SyncServer struct {
	gophkeeperv1.UnimplementedSyncServiceServer
	svc SyncService
}

// NewSyncServer wires a sync Service into gRPC.
func NewSyncServer(svc SyncService) *SyncServer {
	return &SyncServer{svc: svc}
}

// Sync returns item changes since the given cursor.
func (s *SyncServer) Sync(ctx context.Context, req *gophkeeperv1.SyncRequest) (*gophkeeperv1.SyncResponse, error) {
	ownerID, err := requireOwner(ctx)
	if err != nil {
		return nil, err
	}

	var since time.Time
	if ts := req.GetSinceTimestamp(); ts != nil {
		since = ts.AsTime()
	}

	result, err := s.svc.Pull(ctx, ownerID, req.GetSinceVersion(), since)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return gophkeeperv1.SyncResponse_builder{
		Items:         toProtoItems(result.Items),
		ServerVersion: result.ServerVersion,
	}.Build(), nil
}
