package grpc

import (
	"log"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"myaaw/internal/config"
	"myaaw/internal/services/a2a"
)

// GRPCAddr returns the gRPC listen address derived from configuration, or an
// empty string when the binding is disabled.
func GRPCAddr() string {
	addr := strings.TrimSpace(config.A2AGRPCAddr)
	if addr != "" {
		return addr
	}
	if port := strings.TrimSpace(config.A2AGRPCPort); port != "" {
		return ":" + port
	}
	return ""
}

// Serve starts the A2A gRPC server on the configured address, reusing the
// shared service. It returns immediately; the listener runs in a goroutine.
func Serve(service *a2a.Service) {
	addr := GRPCAddr()
	if addr == "" {
		return
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("a2a: gRPC listen on %s: %v", addr, err)
		return
	}

	options := make([]grpc.ServerOption, 0)
	if cert, key := config.A2AGRPCTLSCert, config.A2AGRPCTLSKey; cert != "" && key != "" {
		creds, err := credentials.NewServerTLSFromFile(cert, key)
		if err != nil {
			log.Printf("a2a: gRPC TLS credentials: %v", err)
			_ = listener.Close()
			return
		}
		options = append(options, grpc.Creds(creds))
	}

	server := grpc.NewServer(options...)
	New(service).Register(server)
	go func() {
		if err := server.Serve(listener); err != nil {
			log.Printf("a2a: gRPC server stopped: %v", err)
		}
	}()
	log.Printf("a2a: gRPC server listening on %s", addr)
}
