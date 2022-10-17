package cmd

import (
	"context"
	"fmt"
	"github.com/deliveroo/jing-rpc/internal"
	"github.com/spf13/cobra"
	"os"
	"time"
)

var hopperApp string
var hopperService string
var webPort int
var forwardPort int

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "portal-gun --hopper-app consumer-search-service --hopper-service web --port 3000",
	Args:  cobra.NoArgs,
	Short: "Open a portal from your local machine to staging",
	Long: `Starts an AWS SSM Port Forwarding session to an EC2 instance hosting a task.

You provide the Hopper App and Service name, and portal-gun will track down all of it's associated tasks
and where they're running.`,

	RunE: func(cmd *cobra.Command, args []string) error {

		pg := internal.NewPortalGun()
		ctx := context.Background()

		err := pg.Portal(hopperApp, hopperService, forwardPort)
		if err != nil {

		}

		time.Sleep(10 * time.Second)
		if err := internal.DialGrpcUi(ctx, forwardPort, webPort); err != nil {
			fmt.Printf("failed to dial grpc ui %v", err)
			return err
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ssm-port-forward.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().StringVar(&hopperApp, "hopper-app", "consumer-search-service", "--hopper-app consumer-search-service")
	rootCmd.Flags().StringVar(&hopperService, "hopper-service", "web", "--hopper-service web")
	rootCmd.Flags().IntVar(&webPort, "web-port", 0, "--port 3000 (required)")
	rootCmd.Flags().IntVar(&forwardPort, "forward-port", 0, "--port 3100 (required)")

	if err := rootCmd.MarkFlagRequired("web-port"); err != nil {
		fmt.Fprintf(os.Stderr, "missing argument for web-port: %v\n", err)
		os.Exit(1)
	}
	if err := rootCmd.MarkFlagRequired("forward-port"); err != nil {
		fmt.Fprintf(os.Stderr, "missing argument for forward-port: %v\n", err)
		os.Exit(1)
	}
}
