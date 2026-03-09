package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	var addr string
	var service string
	var timeout time.Duration

	flag.StringVar(&addr, "addr", "localhost:9091", "grpc address")
	flag.StringVar(&service, "service", "proto.catalog.v1.CatalogService", "grpc health service name")
	flag.DurationVar(&timeout, "timeout", 3*time.Second, "request timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: service})
	if err != nil {
		log.Fatalf("health check failed: %v", err)
	}

	fmt.Printf("service=%q status=%s\n", service, resp.GetStatus().String())
}
