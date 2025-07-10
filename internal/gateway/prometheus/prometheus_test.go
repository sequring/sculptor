package prometheus

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

// mockPrometheusAPI is a mock for prometheusv1.API
type mockPrometheusAPI struct {
	queryFunc func(ctx context.Context, query string, ts time.Time, opts ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error)
}

func (m *mockPrometheusAPI) Query(ctx context.Context, query string, ts time.Time, opts ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, ts, opts...)
	}
	return nil, nil, fmt.Errorf("Query not implemented")
}

func (m *mockPrometheusAPI) Alerts(ctx context.Context) (prometheusv1.AlertsResult, error) {
	return prometheusv1.AlertsResult{}, nil
}

func (m *mockPrometheusAPI) AlertManagers(ctx context.Context) (prometheusv1.AlertManagersResult, error) {
	return prometheusv1.AlertManagersResult{}, nil
}

func (m *mockPrometheusAPI) CleanTombstones(ctx context.Context) error {
	return nil
}

func (m *mockPrometheusAPI) Config(ctx context.Context) (prometheusv1.ConfigResult, error) {
	return prometheusv1.ConfigResult{}, nil
}

func (m *mockPrometheusAPI) DeleteSeries(ctx context.Context, matches []string, startTime, endTime time.Time) error {
	return nil
}

func (m *mockPrometheusAPI) Flags(ctx context.Context) (prometheusv1.FlagsResult, error) {
	return prometheusv1.FlagsResult{}, nil
}

func (m *mockPrometheusAPI) LabelNames(ctx context.Context, matches []string, startTime, endTime time.Time, opts ...prometheusv1.Option) ([]string, prometheusv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPrometheusAPI) LabelValues(ctx context.Context, label string, matches []string, startTime, endTime time.Time, opts ...prometheusv1.Option) (model.LabelValues, prometheusv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPrometheusAPI) QueryRange(ctx context.Context, query string, r prometheusv1.Range, opts ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPrometheusAPI) QueryExemplars(ctx context.Context, query string, startTime, endTime time.Time) ([]prometheusv1.ExemplarQueryResult, error) {
	return nil, nil
}

func (m *mockPrometheusAPI) Buildinfo(ctx context.Context) (prometheusv1.BuildinfoResult, error) {
	return prometheusv1.BuildinfoResult{}, nil
}

func (m *mockPrometheusAPI) Runtimeinfo(ctx context.Context) (prometheusv1.RuntimeinfoResult, error) {
	return prometheusv1.RuntimeinfoResult{}, nil
}

func (m *mockPrometheusAPI) Series(ctx context.Context, matches []string, startTime, endTime time.Time, opts ...prometheusv1.Option) ([]model.LabelSet, prometheusv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPrometheusAPI) Snapshot(ctx context.Context, skipHead bool) (prometheusv1.SnapshotResult, error) {
	return prometheusv1.SnapshotResult{}, nil
}

func (m *mockPrometheusAPI) Rules(ctx context.Context) (prometheusv1.RulesResult, error) {
	return prometheusv1.RulesResult{}, nil
}

func (m *mockPrometheusAPI) Targets(ctx context.Context) (prometheusv1.TargetsResult, error) {
	return prometheusv1.TargetsResult{}, nil
}

func (m *mockPrometheusAPI) TargetsMetadata(ctx context.Context, matchTarget, metric, limit string) ([]prometheusv1.MetricMetadata, error) {
	return nil, nil
}

func (m *mockPrometheusAPI) Metadata(ctx context.Context, metric, limit string) (map[string][]prometheusv1.Metadata, error) {
	return nil, nil
}

func (m *mockPrometheusAPI) TSDB(ctx context.Context, opts ...prometheusv1.Option) (prometheusv1.TSDBResult, error) {
	return prometheusv1.TSDBResult{}, nil
}

func (m *mockPrometheusAPI) WalReplay(ctx context.Context) (prometheusv1.WalReplayStatus, error) {
	return prometheusv1.WalReplayStatus{}, nil
}

func TestNewGateway(t *testing.T) {
	logger := slog.Default()
	address := "http://localhost:9090"

	gateway, err := NewGateway(address, logger)

	assert.NoError(t, err)
	assert.NotNil(t, gateway)
	assert.NotNil(t, gateway.api)
	assert.Equal(t, logger, gateway.logger)
}

func TestGateway_GetMemoryMetrics(t *testing.T) {
	tests := []struct {
		name          string
		queryResult   model.Value
		queryError    error
		expectedValue float64
		expectedError string
	}{
		{
			name:          "Successful query",
			queryResult:   model.Vector{ &model.Sample{ Value: 1024 } },
			expectedValue: 1024,
			expectedError: "",
		},
		{
			name:          "Query returns no data",
			queryResult:   model.Vector{},
			expectedValue: 0,
			expectedError: "",
		},
		{
			name:          "Query returns error",
			queryResult:   nil,
			queryError:    fmt.Errorf("network error"),
			expectedValue: 0,
			expectedError: "failed to query Prometheus for P99 Memory Usage on container test-container: network error",
		},
		{
			name:          "Query returns non-vector type",
			queryResult:   &model.String{Value: "test"},
			expectedValue: 0,
			expectedError: "unexpected result type for P99 Memory Usage query: string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockPrometheusAPI{
				queryFunc: func(ctx context.Context, query string, ts time.Time, opts ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error) {
					return tt.queryResult, nil, tt.queryError
				},
			}
			gateway := &Gateway{
				api:    mockAPI,
				logger: slog.Default(),
			}

			value, err := gateway.GetMemoryMetrics(context.Background(), "test-ns", "test-deployment", "test-container", "5m")

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, value)
			}
		})
	}
}
