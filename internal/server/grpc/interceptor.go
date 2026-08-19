package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ctxKey int

const userIDCtxKey ctxKey = 1

// publicMethods are RPCs that do not require a Bearer access token.
var publicMethods = map[string]struct{}{
	"/gophkeeper.v1.AuthService/Register": {},
	"/gophkeeper.v1.AuthService/Login":    {},
	"/gophkeeper.v1.AuthService/Refresh":  {},
	"/gophkeeper.v1.AuthService/Logout":   {},
}

// UserIDFromContext returns the authenticated owner id injected by AuthUnaryInterceptor.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDCtxKey).(string)
	return id, ok && id != ""
}

// ContextWithUserID stores userID in ctx (primarily for tests).
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDCtxKey, userID)
}

// AuthUnaryInterceptor validates JWT access tokens and injects owner_id into context.
func AuthUnaryInterceptor(svc AuthService) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if _, public := publicMethods[info.FullMethod]; public {
			return handler(ctx, req)
		}

		token, err := bearerFromMetadata(ctx)
		if err != nil {
			return nil, err
		}
		userID, err := svc.ParseAccessToken(token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid access token")
		}
		return handler(ContextWithUserID(ctx, userID), req)
	}
}

func bearerFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization")
	}
	parts := strings.SplitN(values[0], " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || parts[1] == "" {
		return "", status.Error(codes.Unauthenticated, "invalid authorization header")
	}
	return parts[1], nil
}
