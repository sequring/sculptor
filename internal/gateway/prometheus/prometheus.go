package prometheus

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Gateway struct {
	api    prometheusv1.API
	silent bool
}

func NewGateway(address string, silent bool) (*Gateway, error) {
	client, err := promapi.NewClient(promapi.Config{Address: address})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}
	return &Gateway{
		api:    prometheusv1.NewAPI(client),
		silent: silent,
	}, nil
}

func (g *Gateway) GetMemoryMetrics(ctx context.Context, ns, name, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.99, sum(container_memory_working_set_bytes{namespace="%s", pod=~"^%s-.*", container!=""}) by (pod, namespace)[%s:]))`, ns, name, timeRange)
	return g.executeQuery(ctx, "P99 Memory Usage", query)
}

func (g *Gateway) GetCPURequestMetrics(ctx context.Context, ns, name, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.90, sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container!=""}[5m])) by (pod, namespace)[%s:1m]))`, ns, name, timeRange)
	return g.executeQuery(ctx, "P90 CPU for Request", query)
}

func (g *Gateway) GetCPULimitMetrics(ctx context.Context, ns, name, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.99, sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container!=""}[5m])) by (pod, namespace)[%s:1m]))`, ns, name, timeRange)
	return g.executeQuery(ctx, "P99 CPU for Limit", query)
}

func (g *Gateway) GetCPUMedianMetrics(ctx context.Context, ns, name, timeRange string) (float64, error) {
	query := fmt.Sprintf(`max(quantile_over_time(0.5, sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container!=""}[5m])) by (pod, namespace)[%s:1m]))`, ns, name, timeRange)
	return g.executeQuery(ctx, "P50 CPU for Spikiness", query)
}

func (g *Gateway) executeQuery(ctx context.Context, queryName string, query string) (float64, error) {
	if !g.silent {
		log.Printf("Fetching %s metrics from Prometheus...", queryName)
	}

	result, warnings, err := g.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to query Prometheus for %s: %w", queryName, err)
	}
	if !g.silent && len(warnings) > 0 {
		log.Printf("Prometheus query for %s returned warnings: %v\n", queryName, warnings)
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected result type for %s: %s", queryName, result.Type().String())
	}

	if vector.Len() == 0 {
		log.Printf("Query for %s returned no data.", queryName)
		return 0, nil
	}

	value := float64(vector[0].Value)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, nil
	}

	return value, nil
}
