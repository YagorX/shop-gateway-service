package catalog_client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	catalogv1 "github.com/YagorX/shop-contracts/gen/go/proto/catalog/v1"
	"github.com/YagorX/shop-gateway/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Client struct {
	log     *slog.Logger
	addr    string
	timeout time.Duration

	conn   *grpc.ClientConn
	client catalogv1.CatalogServiceClient
}

// func NewClient(log *slog.Logger, addr string, timeout time.Duration) (*Client, error) {
// 	if err := validateCatalogClient(addr, timeout, log); err != nil {
// 		return nil, fmt.Errorf("validate catalog client: %w", err)
// 	}

// 	conn, err := grpc.NewClient(
// 		addr,
// 		grpc.WithTransportCredentials(insecure.NewCredentials()),
// 		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
// 	)
// 	if err != nil {
// 		return nil, fmt.Errorf("create grpc client: %w", err)
// 	}

//		return &Client{
//			log:     log,
//			addr:    addr,
//			timeout: timeout,
//			conn:    conn,
//			client:  catalogv1.NewCatalogServiceClient(conn),
//		}, nil
//	}
func NewClient(log *slog.Logger, addr string, timeout time.Duration) (*Client, error) {
	if err := validateCatalogClient(addr, timeout, log); err != nil {
		return nil, fmt.Errorf("validate catalog client: %w", err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("create grpc client: %w", err)
	}

	return &Client{
		log:     log,
		addr:    addr,
		timeout: timeout,
		conn:    conn,
		client:  catalogv1.NewCatalogServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) ListProducts(
	ctx context.Context,
	limit uint32,
	offset uint32,
) (*catalogv1.ListProductsResponse, error) {
	const op = "client.grpc.catalog.ListProducts"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("ListProducts").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("ListProducts", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.ListProducts(reqCtx, &catalogv1.ListProductsRequest{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) GetProduct(
	ctx context.Context,
	id string,
) (*catalogv1.GetProductResponse, error) {
	const op = "client.grpc.catalog.GetProduct"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("GetProduct").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("GetProduct", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.GetProduct(reqCtx, &catalogv1.GetProductRequest{
		Id: id,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func validateCatalogClient(addr string, timeout time.Duration, logger *slog.Logger) error {
	if addr == "" {
		return errors.New("addr is empty")
	}

	if timeout <= 0 {
		return errors.New("timeout is null")
	}

	if logger == nil {
		return errors.New("logger is null")
	}

	return nil
}

func (c *Client) StreamProducts(
	ctx context.Context,
	limit uint32,
	offset uint32,
) (catalogv1.CatalogService_StreamProductsClient, error) {
	const op = "client.grpc.catalog.StreamProducts"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("StreamProducts").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("StreamProducts", grpcCode).Inc()
	}()

	stream, err := c.client.StreamProducts(ctx, &catalogv1.ListProductsRequest{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return stream, nil
}
