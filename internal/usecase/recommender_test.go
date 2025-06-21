package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/sequring/sculptor/internal/entity"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockDeploymentGateway is a mock implementation of the DeploymentGateway interface.
type mockDeploymentGateway struct {
	deploymentToReturn *appsv1.Deployment
	isOOM              bool
	oomLimit           *resource.Quantity
	errToReturn        error
}

func (m *mockDeploymentGateway) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	if m.errToReturn != nil {
		return nil, m.errToReturn
	}
	return m.deploymentToReturn, nil
}

func (m *mockDeploymentGateway) CheckForOOMKilledEvents(ctx context.Context, d *appsv1.Deployment, targetContainerName string) (bool, string, *resource.Quantity, error) {
	if m.errToReturn != nil {
		return false, "", nil, m.errToReturn
	}
	return m.isOOM, "test-pod-12345", m.oomLimit, nil
}

// mockMetricsGateway is a mock implementation of the MetricsGateway interface.
type mockMetricsGateway struct {
	memValue     float64
	cpuP90Value  float64
	cpuP99Value  float64
	cpuP50Value  float64
	initMemValue float64 // Value for the new method
	errToReturn  error
}

func (m *mockMetricsGateway) GetMemoryMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.memValue, m.errToReturn
}
func (m *mockMetricsGateway) GetCPURequestMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.cpuP90Value, m.errToReturn
}
func (m *mockMetricsGateway) GetCPULimitMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.cpuP99Value, m.errToReturn
}
func (m *mockMetricsGateway) GetCPUMedianMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.cpuP50Value, m.errToReturn
}

// FIX: Added missing GetInitContainerMemoryMetrics to satisfy the interface.
func (m *mockMetricsGateway) GetInitContainerMemoryMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.initMemValue, m.errToReturn
}

// --- Test Suite ---

func TestRecommenderUseCase_CalculateForDeployment(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	mem := func(b int64) *resource.Quantity {
		return resource.NewQuantity(b, resource.BinarySI)
	}
	cpu := func(m int64) *resource.Quantity {
		return resource.NewMilliQuantity(m, resource.DecimalSI)
	}

	mockDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "main-container"},
						{Name: "sidecar-container"},
					},
				},
			},
		},
	}

	// FIX: Moved the args struct definition before its use.
	type args struct {
		namespace           string
		deploymentName      string
		targetContainerName string
		timeRange           string
	}

	tests := []struct {
		name       string
		args       args
		depGateway DeploymentGateway
		metGateway MetricsGateway
		want       []NamedRecommendation
		wantErr    bool
	}{
		{
			name: "Happy Path: Standard analysis for all containers",
			args: args{namespace: "default", deploymentName: "test-app"},
			depGateway: &mockDeploymentGateway{
				deploymentToReturn: mockDeployment,
			},
			metGateway: &mockMetricsGateway{
				memValue:    100 * 1024 * 1024, // 100 MiB
				cpuP90Value: 0.2,               // 200m
				cpuP99Value: 0.4,               // 400m
				cpuP50Value: 0.15,              // 150m (ratio 400/150 = 2.66 > 2.0, so spiky)
			},
			want: []NamedRecommendation{
				{
					ContainerName: "main-container",
					Recommendation: &entity.Recommendation{
						Memory:      mem(120 * 1024 * 1024), // 100 MiB * 1.2 buffer
						IsOOMKilled: false,
						CPU: &entity.CPURecommendation{
							Request:          cpu(200),
							Limit:            cpu(500), // 400m * 1.25 spikiness buffer
							SpikinessWarning: true,
						},
					},
				},
				{
					ContainerName: "sidecar-container",
					Recommendation: &entity.Recommendation{
						Memory:      mem(120 * 1024 * 1024),
						IsOOMKilled: false,
						CPU: &entity.CPURecommendation{
							Request:          cpu(200),
							Limit:            cpu(500),
							SpikinessWarning: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "OOMKilled Event: Memory recommendation is multiplied",
			args: args{namespace: "default", deploymentName: "test-app", targetContainerName: "main-container"},
			depGateway: &mockDeploymentGateway{
				deploymentToReturn: mockDeployment,
				isOOM:              true,
				oomLimit:           mem(256 * 1024 * 1024), // OOM at 256Mi
			},
			metGateway: &mockMetricsGateway{ // Metrics should be ignored for memory
				cpuP90Value: 0.1, // 100m
				cpuP99Value: 0.2, // 200m
				cpuP50Value: 0.1, // 100m (not spiky)
			},
			want: []NamedRecommendation{
				{
					ContainerName: "main-container",
					Recommendation: &entity.Recommendation{
						Memory:      mem(384 * 1024 * 1024), // 256Mi * 1.5
						IsOOMKilled: true,
						CPU: &entity.CPURecommendation{
							Request:          cpu(100),
							Limit:            cpu(200),
							SpikinessWarning: false,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "CPU Spikiness: CPU Limit is increased",
			args: args{namespace: "default", deploymentName: "test-app", targetContainerName: "main-container"},
			depGateway: &mockDeploymentGateway{
				deploymentToReturn: mockDeployment,
			},
			metGateway: &mockMetricsGateway{
				memValue:    100 * 1024 * 1024,
				cpuP90Value: 0.1, // 100m
				cpuP99Value: 0.5, // 500m
				cpuP50Value: 0.1, // 100m (ratio 5.0 > 2.0)
			},
			want: []NamedRecommendation{
				{
					ContainerName: "main-container",
					Recommendation: &entity.Recommendation{
						Memory:      mem(120 * 1024 * 1024),
						IsOOMKilled: false,
						CPU: &entity.CPURecommendation{
							Request:          cpu(100),
							Limit:            cpu(625), // 500m * 1.25
							SpikinessWarning: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "No Metrics Data: Container is skipped",
			args: args{namespace: "default", deploymentName: "test-app", targetContainerName: "main-container"},
			depGateway: &mockDeploymentGateway{
				deploymentToReturn: mockDeployment,
			},
			metGateway: &mockMetricsGateway{ // All metrics are 0
				memValue:    0,
				cpuP90Value: 0,
				cpuP99Value: 0,
				cpuP50Value: 0,
			},
			want:    []NamedRecommendation{}, // Expect an empty slice
			wantErr: false,
		},
		{
			name: "--container Flag: Only specified container is analyzed",
			args: args{namespace: "default", deploymentName: "test-app", targetContainerName: "main-container"},
			depGateway: &mockDeploymentGateway{
				deploymentToReturn: mockDeployment,
			},
			metGateway: &mockMetricsGateway{
				memValue:    50 * 1024 * 1024,
				cpuP90Value: 0.05,
				cpuP99Value: 0.1,
				cpuP50Value: 0.04,
			},
			want: []NamedRecommendation{
				{
					ContainerName: "main-container", // Only this container should be in the result
					Recommendation: &entity.Recommendation{
						Memory:      mem(60 * 1024 * 1024),
						IsOOMKilled: false,
						CPU: &entity.CPURecommendation{
							Request:          cpu(50),
							Limit:            cpu(125), // 100m * 1.25
							SpikinessWarning: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:       "Error: Deployment not found",
			args:       args{namespace: "default", deploymentName: "test-app"},
			depGateway: &mockDeploymentGateway{errToReturn: errors.New("deployment not found")},
			metGateway: &mockMetricsGateway{},
			want:       nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewRecommenderUseCase(tt.depGateway, tt.metGateway, testLogger)
			got, err := uc.CalculateForDeployment(context.Background(), tt.args.namespace, tt.args.deploymentName, tt.args.targetContainerName, "7d")

			if (err != nil) != tt.wantErr {
				t.Errorf("RecommenderUseCase.CalculateForDeployment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("RecommenderUseCase.CalculateForDeployment() got %d recommendations, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i].ContainerName != tt.want[i].ContainerName {
					t.Errorf("ContainerName mismatch at index %d: got %s, want %s", i, got[i].ContainerName, tt.want[i].ContainerName)
				}
				if got[i].Recommendation.IsOOMKilled != tt.want[i].Recommendation.IsOOMKilled {
					t.Errorf("IsOOMKilled mismatch for %s: got %v, want %v", got[i].ContainerName, got[i].Recommendation.IsOOMKilled, tt.want[i].Recommendation.IsOOMKilled)
				}
				if got[i].Recommendation.CPU.SpikinessWarning != tt.want[i].Recommendation.CPU.SpikinessWarning {
					t.Errorf("SpikinessWarning mismatch for %s: got %v, want %v", got[i].ContainerName, got[i].Recommendation.CPU.SpikinessWarning, tt.want[i].Recommendation.CPU.SpikinessWarning)
				}
				if got[i].Recommendation.Memory.String() != tt.want[i].Recommendation.Memory.String() {
					t.Errorf("Memory recommendation mismatch for %s: got %s, want %s", got[i].ContainerName, got[i].Recommendation.Memory.String(), tt.want[i].Recommendation.Memory.String())
				}
				if got[i].Recommendation.CPU.Request.String() != tt.want[i].Recommendation.CPU.Request.String() {
					t.Errorf("CPU Request mismatch for %s: got %s, want %s", got[i].ContainerName, got[i].Recommendation.CPU.Request.String(), tt.want[i].Recommendation.CPU.Request.String())
				}
				if got[i].Recommendation.CPU.Limit.String() != tt.want[i].Recommendation.CPU.Limit.String() {
					t.Errorf("CPU Limit mismatch for %s: got %s, want %s", got[i].ContainerName, got[i].Recommendation.CPU.Limit.String(), tt.want[i].Recommendation.CPU.Limit.String())
				}
			}
		})
	}
}
