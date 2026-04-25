package cart_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	cartv1 "github.com/YagorX/shop-contracts/gen/go/proto/cart/v1"
	"github.com/YagorX/shop-gateway/internal/config"
	"github.com/YagorX/shop-gateway/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Client struct {
	log     *slog.Logger
	addr    string
	timeout time.Duration

	conn   *grpc.ClientConn
	client cartv1.CartServiceClient
}

func NewClient(log *slog.Logger, addr string, timeout time.Duration, tlsCfg config.TLSConfig) (*Client, error) {
	if err := validate(addr, timeout, log); err != nil {
		return nil, fmt.Errorf("validate cart client: %w", err)
	}

	transportCreds, err := buildTransportCredentials(tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("build cart transport credentials: %w", err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		addr,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithBlock(),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial cart grpc: %w", err)
	}

	return &Client{
		log:     log,
		addr:    addr,
		timeout: timeout,
		conn:    conn,
		client:  cartv1.NewCartServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// withUserID добавляет user_id в исходящий gRPC metadata.
// cart-service читает его через metadata.FromIncomingContext на своей стороне.
func withUserID(ctx context.Context, userID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "user_id", userID)
}

func (c *Client) AddItem(ctx context.Context, userID, productID string, quantity int32, priceSnapshot int64) (*cartv1.AddItemResponse, error) {
	const op = "client.grpc.cart.AddItem"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("cart.AddItem").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("cart.AddItem", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(withUserID(ctx, userID), c.timeout)
	defer cancel()

	resp, err := c.client.AddItem(reqCtx, &cartv1.AddItemRequest{
		ProductId:     productID,
		Quantity:      quantity,
		PriceSnapshot: priceSnapshot,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) RemoveItem(ctx context.Context, userID, productID string) (*cartv1.RemoveItemResponse, error) {
	const op = "client.grpc.cart.RemoveItem"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("cart.RemoveItem").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("cart.RemoveItem", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(withUserID(ctx, userID), c.timeout)
	defer cancel()

	resp, err := c.client.RemoveItem(reqCtx, &cartv1.RemoveItemRequest{
		ProductId: productID,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) UpdateItem(ctx context.Context, userID, productID string, quantity int32) (*cartv1.UpdateItemResponse, error) {
	const op = "client.grpc.cart.UpdateItem"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("cart.UpdateItem").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("cart.UpdateItem", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(withUserID(ctx, userID), c.timeout)
	defer cancel()

	resp, err := c.client.UpdateItem(reqCtx, &cartv1.UpdateItemRequest{
		ProductId: productID,
		Quantity:  quantity,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) GetCart(ctx context.Context, userID string) (*cartv1.GetCartResponse, error) {
	const op = "client.grpc.cart.GetCart"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("cart.GetCart").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("cart.GetCart", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(withUserID(ctx, userID), c.timeout)
	defer cancel()

	resp, err := c.client.GetCart(reqCtx, &cartv1.GetCartRequest{})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) ClearCart(ctx context.Context, userID string) (*cartv1.ClearCartResponse, error) {
	const op = "client.grpc.cart.ClearCart"
	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("cart.ClearCart").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("cart.ClearCart", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(withUserID(ctx, userID), c.timeout)
	defer cancel()

	resp, err := c.client.ClearCart(reqCtx, &cartv1.ClearCartRequest{})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func buildTransportCredentials(tlsCfg config.TLSConfig) (credentials.TransportCredentials, error) {
	if !tlsCfg.Enabled {
		return insecure.NewCredentials(), nil
	}

	caPEM, err := os.ReadFile(tlsCfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to parse ca certificate")
	}

	clientCert, err := tls.LoadX509KeyPair(tlsCfg.ClientCertFile, tlsCfg.ClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}

	tlsConfig := &tls.Config{
		RootCAs:      pool,
		ServerName:   tlsCfg.ServerName,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}

	return credentials.NewTLS(tlsConfig), nil
}

func validate(addr string, timeout time.Duration, logger *slog.Logger) error {
	if addr == "" {
		return errors.New("addr is empty")
	}
	if timeout <= 0 {
		return errors.New("timeout must be > 0")
	}
	if logger == nil {
		return errors.New("logger is nil")
	}
	return nil
}
