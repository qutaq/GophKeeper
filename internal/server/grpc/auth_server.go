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

// AuthService is the auth use-cases required by gRPC handlers and the interceptor.
type AuthService interface {
	Register(ctx context.Context, login, password string) (*model.User, error)
	Login(ctx context.Context, login, password string) (*model.User, *auth.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	ParseAccessToken(accessToken string) (string, error)
}

// AuthServer implements gophkeeper.v1.AuthService.
type AuthServer struct {
	gophkeeperv1.UnimplementedAuthServiceServer
	svc AuthService
}

// NewAuthServer wires an auth Service into gRPC.
func NewAuthServer(svc AuthService) *AuthServer {
	return &AuthServer{svc: svc}
}

// Register creates a new account.
func (s *AuthServer) Register(ctx context.Context, req *gophkeeperv1.RegisterRequest) (*gophkeeperv1.RegisterResponse, error) {
	user, err := s.svc.Register(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return gophkeeperv1.RegisterResponse_builder{User: toProtoUser(user)}.Build(), nil
}

// Login authenticates and returns tokens.
func (s *AuthServer) Login(ctx context.Context, req *gophkeeperv1.LoginRequest) (*gophkeeperv1.LoginResponse, error) {
	user, pair, err := s.svc.Login(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return gophkeeperv1.LoginResponse_builder{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         toProtoUser(user),
	}.Build(), nil
}

// Refresh rotates tokens.
func (s *AuthServer) Refresh(ctx context.Context, req *gophkeeperv1.RefreshRequest) (*gophkeeperv1.RefreshResponse, error) {
	pair, err := s.svc.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, mapAuthErr(err)
	}
	return gophkeeperv1.RefreshResponse_builder{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	}.Build(), nil
}

// Logout revokes a refresh token.
func (s *AuthServer) Logout(ctx context.Context, req *gophkeeperv1.LogoutRequest) (*gophkeeperv1.LogoutResponse, error) {
	if err := s.svc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, mapAuthErr(err)
	}
	return gophkeeperv1.LogoutResponse_builder{}.Build(), nil
}

func toProtoUser(u *model.User) *gophkeeperv1.User {
	if u == nil {
		return nil
	}
	b := gophkeeperv1.User_builder{Id: u.ID, Login: u.Login}
	if !u.CreatedAt.IsZero() {
		b.CreatedAt = timestamppb.New(u.CreatedAt)
	}
	if !u.UpdatedAt.IsZero() {
		b.UpdatedAt = timestamppb.New(u.UpdatedAt)
	}
	return b.Build()
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
