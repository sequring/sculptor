package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/sequring/sculptor/internal/config"
	k8s_gateway "github.com/sequring/sculptor/internal/gateway/k8s"
	prom_gateway "github.com/sequring/sculptor/internal/gateway/prometheus"
	"github.com/sequring/sculptor/internal/presenter"
	"github.com/sequring/sculptor/internal/usecase"
	"github.com/spf13/pflag"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Error loading config", "error", err)
		os.Exit(1)
	}

	// Set up logger based on silent flag
	var logger *slog.Logger
	if cfg.Silent {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	// Initialize the YAML presenter
	yamlPresenter := presenter.NewYAMLPresenter(logger)
	yamlPresenter.PrintLogo()

	showVersion, _ := pflag.CommandLine.GetBool("version")
	if showVersion {
		fmt.Printf("sculptor-cli version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built on: %s\n", date)
		fmt.Printf("built by: %s\n", builtBy)
		os.Exit(0)
	}

	// Initialize Kubernetes client
	k8sClient, err := k8s_gateway.NewClient(cfg, logger)
	if err != nil {
		logger.Error("Failed to create Kubernetes client", "error", err)
		os.Exit(1)
	}

	// Start port forwarding
	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	defer close(stopCh)

	go func() {
		err := k8sClient.StartPortForward(logger, cfg.Prometheus.Namespace, cfg.Prometheus.Service, cfg.Prometheus.Port, stopCh, readyCh)
		if err != nil {
			errCh <- fmt.Errorf("port-forwarding failed: %w", err)
		}
	}()

	select {
	case <-readyCh:
		logger.Info("Port-forwarding is ready")
	case <-time.After(30 * time.Second):
		logger.Error("Port-forwarding timed out")
		os.Exit(1)
	case err := <-errCh:
		logger.Error("Error occurred", "error", err)
		os.Exit(1)
	}

	// Initialize Prometheus gateway
	prometheusURL := fmt.Sprintf("http://localhost:%d", cfg.Prometheus.Port)
	promGateway, err := prom_gateway.NewGateway(prometheusURL, logger)
	if err != nil {
		logger.Error("Failed to create Prometheus gateway", "error", err)
		os.Exit(1)
	}

	// Initialize use case
	k8sGateway := k8s_gateway.NewGateway(k8sClient.Clientset, logger)
	recommender := usecase.NewRecommenderUseCase(k8sGateway, promGateway, logger)

	logger.Info("Analyzing deployment", "deployment", cfg.Deployment, "namespace", cfg.Namespace, "timeRange", cfg.Range)

	recommendation, finalContainerName, err := recommender.CalculateForDeployment(
		context.Background(),
		cfg.Namespace,
		cfg.Deployment,
		cfg.Container,
		cfg.Range,
	)
	if err != nil {
		logger.Error("Calculation error", "error", err)
		os.Exit(1)
	}

	output, err := yamlPresenter.Render(recommendation, finalContainerName)
	if err != nil {
		logger.Error("Rendering error", "error", err)
		os.Exit(1)
	}

	// Print the output
	fmt.Println(output)
}
