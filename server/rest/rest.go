package rest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"

	pb "github.com/pmaroli/scheduling-rpc/protobufs"
)

// Start the REST reverse proxy
func Start() error {
	fmt.Println("Starting the reverse proxy")
	// Register gRPC server endpoint
	// Note: Make sure the gRPC server is running properly and accessible
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}

	err := pb.RegisterReservationHandlerFromEndpoint(ctx, mux, "localhost:5001", opts)

	if err != nil {
		panic(err)
	}

	// Start HTTP server (and proxy calls to gRPC server endpoint)
	return http.ListenAndServe(":8080", mux)
}
