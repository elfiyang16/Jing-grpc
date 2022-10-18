package internal

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fullstorydev/grpcui/standalone"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func DialGrpcUi(ctx context.Context, forwardPort, webPort int) error {
	svrAddr := fmt.Sprintf("127.0.0.1:%d", forwardPort)
	fmt.Printf("Dialing remote grpc server on address: %s\n", svrAddr)

	cc, err := grpc.DialContext(ctx, svrAddr, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	if err != nil {
		return err
	}

	fmt.Println("Connected to remote grpc server!")
	h, err := standalone.HandlerViaReflection(ctx, cc, svrAddr)
	if err != nil {
		return err
	}

	fmt.Println("Registering grpcui service")
	serveMux := http.NewServeMux()
	serveMux.Handle("/grpcui/", http.StripPrefix("/grpcui", h))

	fmt.Printf("Serving grpc Web Ui on port: %d\n", webPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", webPort), serveMux)
}
