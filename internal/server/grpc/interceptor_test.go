package grpcserver

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/qutaq/gophkeeper/internal/server/auth"
)

func TestAuthUnaryInterceptor(t *testing.T) {
	t.Parallel()

	key := auth.SigningKey{ID: "v1", Key: bytes32(7)}
	ring, err := auth.NewKeyRing(key)
	if err != nil {
		t.Fatal(err)
	}
	tm, err := auth.NewTokenManager(auth.TokenManagerConfig{Keys: ring})
	if err != nil {
		t.Fatal(err)
	}
	svc := auth.NewService(nil, nil, tm)
	interceptor := AuthUnaryInterceptor(svc)

	_, err = interceptor(context.Background(), nil, &grpc.UnaryServerInfo{
		FullMethod: "/gophkeeper.v1.AuthService/Login",
	}, func(ctx context.Context, _ any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = interceptor(context.Background(), nil, &grpc.UnaryServerInfo{
		FullMethod: "/gophkeeper.v1.DataService/GetItem",
	}, func(ctx context.Context, _ any) (any, error) {
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}

	access, _, err := tm.IssueAccess("owner-1")
	if err != nil {
		t.Fatal(err)
	}
	md := metadata.Pairs("authorization", "Bearer "+access)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	var gotUID string
	_, err = interceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/gophkeeper.v1.DataService/GetItem",
	}, func(ctx context.Context, _ any) (any, error) {
		gotUID, _ = UserIDFromContext(ctx)
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotUID != "owner-1" {
		t.Fatalf("uid=%s", gotUID)
	}
}

func bytes32(v byte) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = v
	}
	return b
}
