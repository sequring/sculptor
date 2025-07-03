package prometheus

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Gateway struct {
	api    prometheusv1.API
	logger *slog.Logger
}

func NewGateway(address string, logger *slog.Logger) (*Gateway, error) {
	client, err := promapi.NewClient(promapi.Config{Address: address})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}
	return &Gateway{
		api:    prometheusv1.NewAPI(client),
		logger: logger,
	}, nil
}

func (g *Gateway) GetMemoryMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.99, container_memory_working_set_bytes{namespace="%s", pod=~"^%s-.*", container="%s"}[%s:]))`, ns, deploymentName, containerName, timeRange)
	return g.executeQuery(ctx, "P99 Memory Usage", query, containerName)
}

func (g *Gateway) GetCPURequestMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.90, rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container="%s"}[5m])[%s:1m]))`, ns, deploymentName, containerName, timeRange)
	return g.executeQuery(ctx, "P90 CPU for Request", query, containerName)
}

func (g *Gateway) GetCPULimitMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.99, rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container="%s"}[5m])[%s:1m]))`, ns, deploymentName, containerName, timeRange)
	return g.executeQuery(ctx, "P99 CPU for Limit", query, containerName)
}

func (g *Gateway) GetCPUMedianMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.50, rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container="%s"}[5m])[%s:1m]))`, ns, deploymentName, containerName, timeRange)
	return g.executeQuery(ctx, "P50 CPU for Spikiness", query, containerName)
}

func (g *Gateway) GetInitContainerMemoryMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max_over_time(container_memory_max_usage_bytes{namespace="%s", pod=~"^%s-.*", container="%s"}[%s])`, ns, deploymentName, containerName, timeRange)
	return g.executeQuery(ctx, "Max Memory Usage for Init Container", query, containerName)
}

func (g *Gateway) GetMemoryStdDevMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	query := fmt.Sprintf(`stddev_over_time(container_memory_working_set_bytes{namespace="%s", pod=~"^%s-.*", container="%s"}[%s])`, ns, deploymentName, containerName, timeRange)
	return g.executeQuery(ctx, "Memory StdDev", query, containerName)
}

func (g *Gateway) executeQuery(ctx context.Context, queryName string, query string, containerName string) (float64, error) {
	g.logger.Debug("Fetching metrics from Prometheus", "queryName", queryName, "container", containerName, "query", query)

	result, warnings, err := g.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to query Prometheus for %s on container %s: %w", queryName, containerName, err)
	}
	if len(warnings) > 0 {
		g.logger.Warn("Prometheus query returned warnings", "queryName", queryName, "container", containerName, "warnings", warnings)
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected result type for %s query: %s", queryName, result.Type().String())
	}

	if vector.Len() == 0 {
		g.logger.Info("Query returned no data", "queryName", queryName, "container", containerName)
		return 0, nil
	}

	value := float64(vector[0].Value)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		g.logger.Warn("Query returned non-numeric value (NaN or Inf), treating as 0", "queryName", queryName, "container", containerName)
		return 0, nil
	}

	return value, nil
}
