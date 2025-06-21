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
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Error loading config", "error", err)
		os.Exit(1)
	}

	var logger *slog.Logger
	logLevel := slog.LevelInfo
	if cfg.Verbose {
		logLevel = slog.LevelDebug
	}

	if cfg.Silent {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	} else {
		handlerOpts := &slog.HandlerOptions{Level: logLevel}
		logger = slog.New(slog.NewTextHandler(os.Stderr, handlerOpts))
	}

	presenter.PrintLogo(cfg.Silent)

	showVersion, _ := pflag.CommandLine.GetBool("version")
	if showVersion {
		fmt.Printf("sculptor-cli version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built on: %s\n", date)
		fmt.Printf("built by: %s\n", builtBy)
		os.Exit(0)
	}

	var prometheusURL string

	if cfg.Prometheus.URL != "" {
		prometheusURL = cfg.Prometheus.URL
		logger.Info("Connecting to Prometheus directly", "url", prometheusURL)
	} else {
		logger.Info("Prometheus URL not specified, starting automatic port-forward")
		k8sClientForPF, err := k8s_gateway.NewClient(cfg, logger)
		if err != nil {
			logger.Error("Failed to create Kubernetes client for port-forward", "error", err)
			os.Exit(1)
		}
		stopCh := make(chan struct{}, 1)
		readyCh := make(chan struct{})
		errCh := make(chan error, 1)
		defer close(stopCh)

		go func() {
			err := k8sClientForPF.StartPortForward(logger, cfg.Prometheus.Namespace, cfg.Prometheus.Service, cfg.Prometheus.Port, stopCh, readyCh)
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
			logger.Error("Error during port-forward setup", "error", err)
			os.Exit(1)
		}
		prometheusURL = fmt.Sprintf("http://localhost:%d", cfg.Prometheus.Port)
	}

	k8sClient, err := k8s_gateway.NewClient(cfg, logger)
	if err != nil {
		logger.Error("Failed to create Kubernetes client", "error", err)
		os.Exit(1)
	}
	promGateway, err := prom_gateway.NewGateway(prometheusURL, logger)
	if err != nil {
		logger.Error("Failed to create Prometheus gateway", "error", err)
		os.Exit(1)
	}

	k8sGateway := k8s_gateway.NewGateway(k8sClient.Clientset, logger)
	recommender := usecase.NewRecommenderUseCase(k8sGateway, promGateway, logger)
	yamlPresenter := presenter.NewYAMLPresenter(cfg.Silent, os.Stdout)

	switch cfg.Target {
	case "all":
		logger.Info("Analyzing all containers", "deployment", cfg.Deployment, "namespace", cfg.Namespace, "range", cfg.Range)
		recommendations, err := recommender.CalculateForAll(context.Background(), cfg.Namespace, cfg.Deployment, cfg.Container, cfg.Range)
		if err != nil {
			logger.Error("Error calculating recommendations", "error", err)
			os.Exit(1)
		}
		err = yamlPresenter.RenderAll(recommendations)
		if err != nil {
			logger.Error("Error rendering recommendations", "error", err)
			os.Exit(1)
		}

	case "init":
		logger.Info("Analyzing init containers", "deployment", cfg.Deployment, "namespace", cfg.Namespace, "range", cfg.Range)
		recommendations, err := recommender.CalculateForInitContainers(context.Background(), cfg.Namespace, cfg.Deployment, cfg.Container, cfg.Range)
		if err != nil {
			logger.Error("Init container calculation error", "error", err)
			os.Exit(1)
		}
		err = yamlPresenter.RenderInit(recommendations)
		if err != nil {
			logger.Error("Init container rendering error", "error", err)
			os.Exit(1)
		}

	default: // main
		logger.Info("Analyzing main containers", "deployment", cfg.Deployment, "namespace", cfg.Namespace, "range", cfg.Range)
		recommendations, err := recommender.CalculateForDeployment(context.Background(), cfg.Namespace, cfg.Deployment, cfg.Container, cfg.Range)
		if err != nil {
			logger.Error("Main container calculation error", "error", err)
			os.Exit(1)
		}
		err = yamlPresenter.Render(recommendations)
		if err != nil {
			logger.Error("Main container rendering error", "error", err)
			os.Exit(1)
		}
	}
}
