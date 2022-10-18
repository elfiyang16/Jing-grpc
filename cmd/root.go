package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/deliveroo/jing-rpc/internal"
)

var hopperApp string
var hopperService string
var webPort int
var forwardPort int

var rootCmd = &cobra.Command{
	Use:   "jing-rpc --hopper-app xxxx --hopper-service xxx --web-port 3000 -forward-port 3001",
	Args:  cobra.NoArgs,
	Short: "Open a Grpcui GUI and start testing by portal forwarding your local machine to staging EC2",
	Long: `Open a Grpcui GUI (https://github.com/fullstorydev/grpcui) and connects to your grpc service on staging.
This is done by through opening a SSM Port Forwarding session to an EC2 instance hosting a task.
You provide the Hopper App and Service name, picking the task container, and we will forward the local port to the remote one, 
and register the grpc service to Grpcui.`,

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pg := internal.NewPortalGun()

		msgStream, err := pg.Portal(ctx, hopperApp, hopperService, forwardPort)
		if err != nil {
			fmt.Printf("Failed to establish session manager port forwarding: %v", err)
			return err
		}
		// est. time for SSM starts up the session. Alternatively can listen to this signal:
		// https://github.com/aws/session-manager-plugin/blob/c523002ee02c8b68983ad05042ed52c44d867952/src/sessionmanagerplugin/session/portsession/muxportforwarding.go#L257
		// But it's more fragile
		go func() {
			for msg := range msgStream {
				fmt.Printf("SSM: %s\n", msg)
			}
			fmt.Println("Stop reading")
		}()
		time.Sleep(10 * time.Second)

		if err := internal.DialGrpcUi(ctx, forwardPort, webPort); err != nil {
			fmt.Printf("Failed to dial grpc ui %v", err)
			return err
		}
		return nil
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVar(&hopperApp, "hopper-app", "", "--hopper-app app-name(required)")
	rootCmd.Flags().StringVar(&hopperService, "hopper-service", "", "--hopper-service grpc(required)")
	rootCmd.Flags().IntVar(&webPort, "web-port", 0, "--port 3000 (required)")
	rootCmd.Flags().IntVar(&forwardPort, "forward-port", 0, "--port 3100 (required)")

	if err := rootCmd.MarkFlagRequired("hopper-app"); err != nil {
		fmt.Fprintf(os.Stderr, "missing argument for hopper-app: %v\n", err)
		os.Exit(1)
	}
	if err := rootCmd.MarkFlagRequired("hopper-service"); err != nil {
		fmt.Fprintf(os.Stderr, "missing argument for hopper-service: %v\n", err)
		os.Exit(1)
	}
	if err := rootCmd.MarkFlagRequired("web-port"); err != nil {
		fmt.Fprintf(os.Stderr, "missing argument for web-port: %v\n", err)
		os.Exit(1)
	}
	if err := rootCmd.MarkFlagRequired("forward-port"); err != nil {
		fmt.Fprintf(os.Stderr, "missing argument for forward-port: %v\n", err)
		os.Exit(1)
	}
}
