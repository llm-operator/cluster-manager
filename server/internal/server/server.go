package server

import (
	"context"
	"fmt"
	"log"
	"net"

	v1 "github.com/llm-operator/cluster-manager/api/v1"
	"github.com/llm-operator/cluster-manager/server/internal/config"
	"github.com/llm-operator/cluster-manager/server/internal/store"
	"github.com/llm-operator/rbac-manager/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	defaultProjectID = "default"
	defaultTenantID  = "default-tenant-id"
)

// New creates a server.
func New(store *store.S) *S {
	return &S{
		store: store,
	}
}

// S is a server.
type S struct {
	v1.UnimplementedClustersServiceServer

	srv *grpc.Server

	store *store.S

	enableAuth bool
}

// Run starts the gRPC server.
func (s *S) Run(ctx context.Context, port int, authConfig config.AuthConfig) error {
	log.Printf("Starting server on port %d\n", port)

	var opts []grpc.ServerOption
	if authConfig.Enable {
		ai, err := auth.NewInterceptor(ctx, auth.Config{
			RBACServerAddr: authConfig.RBACInternalServerAddr,
			AccessResource: "api.clusters",
		})
		if err != nil {
			return err
		}
		opts = append(opts, grpc.ChainUnaryInterceptor(ai.Unary()))
		s.enableAuth = true
	}

	grpcServer := grpc.NewServer(opts...)
	v1.RegisterClustersServiceServer(grpcServer, s)
	reflection.Register(grpcServer)

	s.srv = grpcServer

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("listen: %s", err)
	}
	if err := grpcServer.Serve(l); err != nil {
		return fmt.Errorf("serve: %s", err)
	}
	return nil
}

// Stop stops the gRPC server.
func (s *S) Stop() {
	s.srv.Stop()
}

func (s *S) extractUserInfoFromContext(ctx context.Context) (*auth.UserInfo, error) {
	if !s.enableAuth {
		return &auth.UserInfo{
			OrganizationID: "default",
			ProjectID:      defaultProjectID,
			AssignedKubernetesEnvs: []auth.AssignedKubernetesEnv{
				{
					ClusterID: "default",
					Namespace: "default",
				},
			},
			TenantID: defaultTenantID,
		}, nil
	}
	var ok bool
	userInfo, ok := auth.ExtractUserInfoFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "user info not found")
	}
	return userInfo, nil
}
