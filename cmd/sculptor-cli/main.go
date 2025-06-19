package main

import (
	"context"
	"fmt"
	"log"
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
	yamlPresenter := presenter.NewYAMLPresenter()
	yamlPresenter.PrintLogo()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	showVersion, _ := pflag.CommandLine.GetBool("version")
	if showVersion {
		fmt.Printf("sculptor-cli version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built on: %s\n", date)
		fmt.Printf("built by: %s\n", builtBy)
		os.Exit(0)
	}

	if cfg == nil {
		log.Fatal("Could not load configuration.")
	}

	k8sClient, err := k8s_gateway.NewClient(cfg)
	if err != nil {
		log.Fatalf("K8s client error: %v", err)
	}

	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	defer close(stopCh)

	go func() {
		err := k8sClient.StartPortForward(cfg.Prometheus.Namespace, cfg.Prometheus.Service, cfg.Prometheus.Port, stopCh, readyCh)
		if err != nil {
			errCh <- fmt.Errorf("port-forwarding failed: %w", err)
		}
	}()

	select {
	case <-readyCh:
		log.Println("Port-forwarding is ready.")
	case <-time.After(30 * time.Second):
		log.Fatal("Port-forwarding timed out.")
	case err := <-errCh:
		log.Fatalf("Error occurred: %v", err)
	}

	prometheusURL := fmt.Sprintf("http://localhost:%d", cfg.Prometheus.Port)
	promGateway, err := prom_gateway.NewGateway(prometheusURL)
	if err != nil {
		log.Fatalf("Prometheus client error: %v", err)
	}

	k8sGateway := k8s_gateway.NewGateway(k8sClient.Clientset)
	recommender := usecase.NewRecommenderUseCase(k8sGateway, promGateway)

	log.Printf("Analyzing deployment '%s' in namespace '%s' over the last %s...\n", cfg.Deployment, cfg.Namespace, cfg.Range)

	recommendation, finalContainerName, err := recommender.CalculateForDeployment(
		context.Background(),
		cfg.Namespace,
		cfg.Deployment,
		cfg.Container,
		cfg.Range,
	)
	if err != nil {
		log.Fatalf("Calculation error: %v", err)
	}

	output, err := yamlPresenter.Render(recommendation, finalContainerName)
	if err != nil {
		log.Fatalf("Rendering error: %v", err)
	}

	fmt.Println(output)
}
