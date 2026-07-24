package grpcserver

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/qutaq/gophkeeper/internal/model"
	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
	"github.com/qutaq/gophkeeper/internal/server/data"
	"github.com/qutaq/gophkeeper/internal/server/storage"
)

// DataServer implements gophkeeper.v1.DataService.
type DataServer struct {
	gophkeeperv1.UnimplementedDataServiceServer
	svc *data.Service
}

// NewDataServer wires a data Service into gRPC.
func NewDataServer(svc *data.Service) *DataServer {
	return &DataServer{svc: svc}
}

// AddItem creates an encrypted item for the authenticated owner.
func (s *DataServer) AddItem(ctx context.Context, req *gophkeeperv1.AddItemRequest) (*gophkeeperv1.AddItemResponse, error) {
	ownerID, err := requireOwner(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.svc.Add(ctx, ownerID, data.AddInput{
		Type:          model.DataType(req.GetType()),
		EncryptedData: req.GetEncryptedData(),
		Metadata:      fromProtoMetadata(req.GetMetadata()),
	})
	if err != nil {
		return nil, mapDataErr(err)
	}
	return &gophkeeperv1.AddItemResponse{Item: toProtoItem(item)}, nil
}

// UpdateItem updates an owned item.
func (s *DataServer) UpdateItem(ctx context.Context, req *gophkeeperv1.UpdateItemRequest) (*gophkeeperv1.UpdateItemResponse, error) {
	ownerID, err := requireOwner(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.svc.Update(ctx, ownerID, data.UpdateInput{
		ID:              req.GetId(),
		EncryptedData:   req.GetEncryptedData(),
		Metadata:        fromProtoMetadata(req.GetMetadata()),
		ExpectedVersion: req.GetExpectedVersion(),
	})
	if err != nil {
		return nil, mapDataErr(err)
	}
	return &gophkeeperv1.UpdateItemResponse{Item: toProtoItem(item)}, nil
}

// DeleteItem soft-deletes an owned item.
func (s *DataServer) DeleteItem(ctx context.Context, req *gophkeeperv1.DeleteItemRequest) (*gophkeeperv1.DeleteItemResponse, error) {
	ownerID, err := requireOwner(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.svc.Delete(ctx, ownerID, req.GetId())
	if err != nil {
		return nil, mapDataErr(err)
	}
	return &gophkeeperv1.DeleteItemResponse{Item: toProtoItem(item)}, nil
}

// ListItems lists items for the authenticated owner.
func (s *DataServer) ListItems(ctx context.Context, req *gophkeeperv1.ListItemsRequest) (*gophkeeperv1.ListItemsResponse, error) {
	ownerID, err := requireOwner(ctx)
	if err != nil {
		return nil, err
	}
	filter := storage.ListItemsFilter{IncludeDeleted: req.GetIncludeDeleted()}
	if req.GetType() != gophkeeperv1.DataType_DATA_TYPE_UNSPECIFIED {
		t := model.DataType(req.GetType())
		filter.Type = &t
	}
	items, err := s.svc.List(ctx, ownerID, filter)
	if err != nil {
		return nil, mapDataErr(err)
	}
	return &gophkeeperv1.ListItemsResponse{Items: toProtoItems(items)}, nil
}

// GetItem returns one owned item.
func (s *DataServer) GetItem(ctx context.Context, req *gophkeeperv1.GetItemRequest) (*gophkeeperv1.GetItemResponse, error) {
	ownerID, err := requireOwner(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.svc.Get(ctx, ownerID, req.GetId())
	if err != nil {
		return nil, mapDataErr(err)
	}
	return &gophkeeperv1.GetItemResponse{Item: toProtoItem(item)}, nil
}

func requireOwner(ctx context.Context) (string, error) {
	ownerID, ok := UserIDFromContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "unauthenticated")
	}
	return ownerID, nil
}

func mapDataErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, data.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, data.ErrNotFound):
		return status.Error(codes.NotFound, "item not found")
	case errors.Is(err, data.ErrConflict):
		return status.Error(codes.Aborted, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
