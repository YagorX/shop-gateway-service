package handlers

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type AuthHealthChecker struct {
	Addr    string
	Timeout time.Duration
}

func (c AuthHealthChecker) Check(ctx context.Context) error {
	if c.Addr == "" {
		return fmt.Errorf("auth grpc addr is empty")
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := grpc.DialContext(
		checkCtx,
		c.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("dial auth grpc: %w", err)
	}
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	resp, err := client.Check(checkCtx, &healthpb.HealthCheckRequest{
		Service: "proto.auth.v1.AuthService",
	})
	if err != nil {
		return fmt.Errorf("auth health check failed: %w", err)
	}

	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("auth not serving: %s", resp.GetStatus().String())
	}

	return nil
}
