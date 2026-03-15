package auth_client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	authv1 "github.com/YagorX/shop-contracts/gen/go/proto/auth/v1"
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
	client authv1.AuthServiceClient
}

// func NewClient(log *slog.Logger, addr string, timeout time.Duration) (*Client, error) {
// 	if err := validateAuthClient(addr, timeout, log); err != nil {
// 		return nil, fmt.Errorf("validate auth client: %w", err)
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
//			client:  authv1.NewAuthServiceClient(conn),
//		}, nil
//	}
func NewClient(log *slog.Logger, addr string, timeout time.Duration) (*Client, error) {
	if err := validateAuthClient(addr, timeout, log); err != nil {
		return nil, fmt.Errorf("validate auth client: %w", err)
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
		client:  authv1.NewAuthServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Register(
	ctx context.Context,
	username string,
	email string,
	password string,
) (*authv1.RegisterResponse, error) {
	const op = "client.grpc.auth.Register"

	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("AuthRegister").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("AuthRegister", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Register(reqCtx, &authv1.RegisterRequest{
		Username: username,
		Email:    email,
		Password: password,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) Login(
	ctx context.Context,
	emailOrName string,
	password string,
	appID int64,
	deviceID string,
) (*authv1.LoginResponse, error) {
	const op = "client.grpc.auth.Login"

	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("AuthLogin").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("AuthLogin", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Login(reqCtx, &authv1.LoginRequest{
		EmailOrName: emailOrName,
		Password:    password,
		AppId:       appID,
		DeviceId:    deviceID,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) ValidateToken(
	ctx context.Context,
	token string,
	appID int64,
) (*authv1.ValidateTokenResponse, error) {
	const op = "client.grpc.auth.ValidateToken"

	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("AuthValidateToken").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("AuthValidateToken", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.ValidateToken(reqCtx, &authv1.ValidateTokenRequest{
		Token: token,
		AppId: appID,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) Refresh(
	ctx context.Context,
	refreshToken string,
	appID int64,
	deviceID string,
) (*authv1.RefreshResponse, error) {
	const op = "client.grpc.auth.Refresh"

	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("AuthRefresh").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("AuthRefresh", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Refresh(reqCtx, &authv1.RefreshRequest{
		RefreshToken: refreshToken,
		AppId:        appID,
		DeviceId:     deviceID,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) Logout(
	ctx context.Context,
	refreshToken string,
	appID int64,
	deviceID string,
) (*authv1.LogoutResponse, error) {
	const op = "client.grpc.auth.Logout"

	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("AuthLogout").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("AuthLogout", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Logout(reqCtx, &authv1.LogoutRequest{
		RefreshToken: refreshToken,
		AppId:        appID,
		DeviceId:     deviceID,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func (c *Client) IsAdmin(
	ctx context.Context,
	userUUID string,
) (*authv1.IsAdminResponse, error) {
	const op = "client.grpc.auth.IsAdmin"

	startedAt := time.Now()
	metrics := observability.MustMetrics()
	grpcCode := codes.OK.String()
	defer func() {
		metrics.GatewayGRPCRequestDuration.WithLabelValues("AuthIsAdmin").Observe(time.Since(startedAt).Seconds())
		metrics.GatewayGRPCRequestsTotal.WithLabelValues("AuthIsAdmin", grpcCode).Inc()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.IsAdmin(reqCtx, &authv1.IsAdminRequest{
		UserUuid: userUUID,
	})
	if err != nil {
		grpcCode = status.Code(err).String()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resp, nil
}

func validateAuthClient(addr string, timeout time.Duration, logger *slog.Logger) error {
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
