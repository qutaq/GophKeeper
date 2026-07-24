// Package grpcserver implements gRPC handlers and interceptors for the server.
package grpcserver

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/qutaq/gophkeeper/internal/model"
	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
	"github.com/qutaq/gophkeeper/internal/server/auth"
)

// AuthServer implements gophkeeper.v1.AuthService.
type AuthServer struct {
	gophkeeperv1.UnimplementedAuthServiceServer
	svc *auth.Service
}

// NewAuthServer wires an auth Service into gRPC.
func NewAuthServer(svc *auth.Service) *AuthServer {
	return &AuthServer{svc: svc}
}

// Register creates a new account.
func (s *AuthServer) Register(ctx context.Context, req *gophkeeperv1.RegisterRequest) (*gophkeeperv1.RegisterResponse, error) {
	user, err := s.svc.Register(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return &gophkeeperv1.RegisterResponse{User: toProtoUser(user)}, nil
}

// Login authenticates and returns tokens.
func (s *AuthServer) Login(ctx context.Context, req *gophkeeperv1.LoginRequest) (*gophkeeperv1.LoginResponse, error) {
	user, pair, err := s.svc.Login(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return &gophkeeperv1.LoginResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         toProtoUser(user),
	}, nil
}

// Refresh rotates tokens.
func (s *AuthServer) Refresh(ctx context.Context, req *gophkeeperv1.RefreshRequest) (*gophkeeperv1.RefreshResponse, error) {
	pair, err := s.svc.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return &gophkeeperv1.RefreshResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	}, nil
}

// Logout revokes a refresh token.
func (s *AuthServer) Logout(ctx context.Context, req *gophkeeperv1.LogoutRequest) (*gophkeeperv1.LogoutResponse, error) {
	if err := s.svc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, mapAuthErr(err)
	}
	return &gophkeeperv1.LogoutResponse{}, nil
}

func toProtoUser(u *model.User) *gophkeeperv1.User {
	if u == nil {
		return nil
	}
	out := &gophkeeperv1.User{Id: u.ID, Login: u.Login}
	if !u.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(u.CreatedAt)
	}
	if !u.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(u.UpdatedAt)
	}
	return out
}

func mapAuthErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, auth.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, auth.ErrLoginTaken):
		return status.Error(codes.AlreadyExists, "login already taken")
	case errors.Is(err, auth.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case errors.Is(err, auth.ErrInvalidToken):
		return status.Error(codes.Unauthenticated, "invalid token")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
