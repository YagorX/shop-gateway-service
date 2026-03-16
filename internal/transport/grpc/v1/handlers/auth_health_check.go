package handlers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type TLSConfig struct {
	Enabled        bool
	CAFile         string
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string
}

type AuthHealthChecker struct {
	Addr    string
	Timeout time.Duration
	TLS     TLSConfig
}

func (c AuthHealthChecker) Check(ctx context.Context) error {
	if c.Addr == "" {
		return fmt.Errorf("auth grpc addr is empty")
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	transportCreds, err := buildAuthTransportCredentials(c.TLS)
	if err != nil {
		return fmt.Errorf("build auth health transport credentials: %w", err)
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := grpc.DialContext(
		checkCtx,
		c.Addr,
		grpc.WithTransportCredentials(transportCreds),
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

func buildAuthTransportCredentials(tlsCfg TLSConfig) (credentials.TransportCredentials, error) {
	if !tlsCfg.Enabled {
		return insecure.NewCredentials(), nil
	}

	caPEM, err := os.ReadFile(tlsCfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read ca file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("append ca cert to pool")
	}

	clientCert, err := tls.LoadX509KeyPair(tlsCfg.ClientCertFile, tlsCfg.ClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		RootCAs:      pool,
		ServerName:   tlsCfg.ServerName,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}

	return credentials.NewTLS(tlsConfig), nil
}
