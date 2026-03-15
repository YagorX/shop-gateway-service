package handlers

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type CatalogHealthChecker struct {
	Addr    string
	Timeout time.Duration
}

// func (c CatalogHealthChecker) Check(ctx context.Context) error {
// 	if c.Addr == "" {
// 		return fmt.Errorf("catalog grpc addr is empty")
// 	}

// 	timeout := c.Timeout
// 	if timeout <= 0 {
// 		timeout = 2 * time.Second
// 	}

// 	checkCtx, cancel := context.WithTimeout(ctx, timeout)
// 	defer cancel()

// 	conn, err := grpc.NewClient(
// 		c.Addr,
// 		grpc.WithTransportCredentials(insecure.NewCredentials()),
// 	)

// 	if err != nil {
// 		return fmt.Errorf("create grpc client: %w", err)
// 	}
// 	defer conn.Close()

// 	// conn.Connect()

// 	client := healthpb.NewHealthClient(conn)
// 	resp, err := client.Check(checkCtx, &healthpb.HealthCheckRequest{Service: "proto.catalog.v1.CatalogService"})
// 	if err != nil {
// 		return fmt.Errorf("catalog health check failed: %w", err)
// 	}

// 	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
// 		return fmt.Errorf("catalog not serving: %s", resp.GetStatus().String())
// 	}

// 	return nil
// }

func (c CatalogHealthChecker) Check(ctx context.Context) error {
	if c.Addr == "" {
		return fmt.Errorf("catalog grpc addr is empty")
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
		return fmt.Errorf("dial catalog grpc: %w", err)
	}
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	resp, err := client.Check(checkCtx, &healthpb.HealthCheckRequest{
		Service: "proto.catalog.v1.CatalogService",
	})
	if err != nil {
		return fmt.Errorf("catalog health check failed: %w", err)
	}

	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("catalog not serving: %s", resp.GetStatus().String())
	}

	return nil
}
