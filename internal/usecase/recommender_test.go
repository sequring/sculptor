// File: internal/usecase/recommender_test.go

package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/sequring/sculptor/internal/entity"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Mock Implementations ---

type mockDeploymentGateway struct {
	deployment       *appsv1.Deployment
	getDeploymentErr error
	isOOMKilled      bool
	oomPodName       string
	oomCurrentLimit  *resource.Quantity
	checkOOMErr      error
}

func (m *mockDeploymentGateway) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	if m.getDeploymentErr != nil {
		return nil, m.getDeploymentErr
	}
	return m.deployment, nil
}

func (m *mockDeploymentGateway) CheckForOOMKilledEvents(ctx context.Context, d *appsv1.Deployment, targetContainerName string) (bool, string, *resource.Quantity, error) {
	if m.checkOOMErr != nil {
		return false, "", nil, m.checkOOMErr
	}
	// Allow specific container targeting for OOM tests
	if m.isOOMKilled && targetContainerName == m.oomPodName {
		return true, m.oomPodName, m.oomCurrentLimit, m.checkOOMErr
	}
	return false, "", nil, m.checkOOMErr
}

type mockMetricsGateway struct {
	memValue          float64
	cpuP90Value       float64
	cpuP99Value       float64
	cpuP50Value       float64
	initMemValue      float64
	getMetricsErr     error
	getInitMetricsErr error
}

func (m *mockMetricsGateway) GetMemoryMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.memValue, m.getMetricsErr
}
func (m *mockMetricsGateway) GetCPURequestMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.cpuP90Value, m.getMetricsErr
}
func (m *mockMetricsGateway) GetCPULimitMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.cpuP99Value, m.getMetricsErr
}
func (m *mockMetricsGateway) GetCPUMedianMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.cpuP50Value, m.getMetricsErr
}
func (m *mockMetricsGateway) GetInitContainerMemoryMetrics(ctx context.Context, ns, deploymentName, containerName, timeRange string) (float64, error) {
	return m.initMemValue, m.getInitMetricsErr
}

// --- Helper Functions ---

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustParseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func quantityFromInt(val int64) *resource.Quantity {
	return resource.NewQuantity(val, resource.BinarySI)
}

// --- Test Suite ---

func TestRecommenderUseCase_CalculateForAll(t *testing.T) {
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers:     []v1.Container{{Name: "main-app"}},
					InitContainers: []v1.Container{{Name: "init-setup"}},
				},
			},
		},
	}

	tests := []struct {
		name         string
		deploymentGW DeploymentGateway
		metricsGW    MetricsGateway
		target       string
		container    string
		want         *AllRecommendations
		wantErr      bool
	}{
		{
			name:         "Happy Path: all containers with metrics above floor",
			target:       "all",
			deploymentGW: &mockDeploymentGateway{deployment: baseDeployment},
			metricsGW: &mockMetricsGateway{
				memValue:     100 * 1024 * 1024,
				cpuP90Value:  0.2,
				cpuP99Value:  0.4,
				cpuP50Value:  0.25,
				initMemValue: 50 * 1024 * 1024,
			},
			want: &AllRecommendations{
				MainContainers: []NamedRecommendation{{
					ContainerName: "main-app",
					Recommendation: &entity.Recommendation{
						Memory: quantityFromInt((100 * 1024 * 1024 * mainContainerMemoryBufferPercent) / 100),
						CPU:    &entity.CPURecommendation{Request: mustParseQuantity("200m"), Limit: mustParseQuantity("400m")},
					},
				}},
				InitContainers: []NamedRecommendation{{
					ContainerName: "init-setup",
					Recommendation: &entity.Recommendation{
						Memory: quantityFromInt((50 * 1024 * 1024 * initContainerMemoryBufferPercent) / 100),
						CPU:    &entity.CPURecommendation{Request: mustParseQuantity(initCPURequestDefault), Limit: mustParseQuantity(initCPULimitDefault)},
					},
				}},
			},
			wantErr: false,
		},
		{
			name:         "OOMKilled Event: applies OOM multiplier and min floor for CPU",
			target:       "main",
			deploymentGW: &mockDeploymentGateway{deployment: baseDeployment, isOOMKilled: true, oomPodName: "main-app", oomCurrentLimit: mustParseQuantity("256Mi")},
			metricsGW:    &mockMetricsGateway{},
			want: &AllRecommendations{
				MainContainers: []NamedRecommendation{{
					ContainerName: "main-app",
					Recommendation: &entity.Recommendation{
						IsOOMKilled: true,
						Memory:      mustParseQuantity(fmt.Sprintf("%dMi", int(256*oomMemoryMultiplier))),
						CPU:         &entity.CPURecommendation{Request: mustParseQuantity(fmt.Sprintf("%dm", minCPURequestMilli)), Limit: mustParseQuantity(fmt.Sprintf("%dm", minCPULimitMilli))},
					},
				}},
			},
			wantErr: false,
		},
		{
			name:         "CPU Spikiness: CPU limit is buffered and memory is also buffered",
			target:       "main",
			deploymentGW: &mockDeploymentGateway{deployment: baseDeployment},
			metricsGW: &mockMetricsGateway{
				memValue:    80 * 1024 * 1024, // 80Mi (> 64Mi floor)
				cpuP90Value: 0.1, cpuP99Value: 0.5, cpuP50Value: 0.1,
			},
			want: &AllRecommendations{
				MainContainers: []NamedRecommendation{{
					ContainerName: "main-app",
					Recommendation: &entity.Recommendation{
						Memory: quantityFromInt((80 * 1024 * 1024 * mainContainerMemoryBufferPercent) / 100),
						CPU: &entity.CPURecommendation{
							Request:          mustParseQuantity("100m"),
							Limit:            mustParseQuantity(fmt.Sprintf("%dm", int(0.5*spikinessCPUBuffer*1000))),
							SpikinessWarning: true,
						},
					},
				}},
			},
			wantErr: false,
		},
		{
			name:         "Very Low Usage: applies min floor values",
			target:       "main",
			deploymentGW: &mockDeploymentGateway{deployment: baseDeployment},
			metricsGW: &mockMetricsGateway{
				memValue:    10 * 1024 * 1024,
				cpuP90Value: 0.01,
				cpuP99Value: 0.02,
			},
			want: &AllRecommendations{
				MainContainers: []NamedRecommendation{{
					ContainerName: "main-app",
					Recommendation: &entity.Recommendation{
						Memory: quantityFromInt(minMemoryBytes),
						CPU: &entity.CPURecommendation{
							Request: mustParseQuantity(fmt.Sprintf("%dm", minCPURequestMilli)),
							Limit:   mustParseQuantity(fmt.Sprintf("%dm", minCPULimitMilli)),
						},
					},
				}},
			},
			wantErr: false,
		},
		{
			name:         "No Metrics Data: main container gets min floor values",
			target:       "main",
			deploymentGW: &mockDeploymentGateway{deployment: baseDeployment},
			metricsGW:    &mockMetricsGateway{},
			want: &AllRecommendations{
				MainContainers: []NamedRecommendation{{
					ContainerName: "main-app",
					Recommendation: &entity.Recommendation{
						Memory: quantityFromInt(minMemoryBytes),
						CPU: &entity.CPURecommendation{
							Request: mustParseQuantity(fmt.Sprintf("%dm", minCPURequestMilli)),
							Limit:   mustParseQuantity(fmt.Sprintf("%dm", minCPULimitMilli)),
						},
					},
				}},
			},
			wantErr: false,
		},
		{
			name:         "No Init Containers: InitRecs slice is empty",
			target:       "all",
			deploymentGW: &mockDeploymentGateway{deployment: &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: v1.PodTemplateSpec{Spec: v1.PodSpec{Containers: []v1.Container{{Name: "main-app"}}}}}}},
			metricsGW:    &mockMetricsGateway{memValue: minMemoryBytes, cpuP90Value: 0.1, cpuP99Value: 0.1},
			want:         &AllRecommendations{MainContainers: []NamedRecommendation{{ContainerName: "main-app"}}, InitContainers: []NamedRecommendation{}},
			wantErr:      false,
		},
		{
			name:      "--container Flag Usage: only specified container is analyzed",
			target:    "all",
			container: "main-app",
			deploymentGW: &mockDeploymentGateway{
				deployment: &appsv1.Deployment{
					Spec: appsv1.DeploymentSpec{Template: v1.PodTemplateSpec{Spec: v1.PodSpec{Containers: []v1.Container{{Name: "main-app"}, {Name: "sidecar"}}, InitContainers: []v1.Container{{Name: "init-setup"}}}}},
				},
			},
			metricsGW: &mockMetricsGateway{memValue: minMemoryBytes, cpuP90Value: 0.1, cpuP99Value: 0.1},
			want: &AllRecommendations{
				MainContainers: []NamedRecommendation{{ContainerName: "main-app"}},
				InitContainers: []NamedRecommendation{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewRecommenderUseCase(tt.deploymentGW, tt.metricsGW, newTestLogger())
			var got *AllRecommendations
			var err error

			switch tt.target {
			case "main":
				mainRecs, mainErr := uc.CalculateForDeployment(context.Background(), "test-ns", "test-deployment", tt.container, "7d")
				if mainErr == nil {
					got = &AllRecommendations{MainContainers: mainRecs}
				}
				err = mainErr
			case "init":
				initRecs, initErr := uc.CalculateForInitContainers(context.Background(), "test-ns", "test-deployment", tt.container, "7d")
				if initErr == nil {
					got = &AllRecommendations{InitContainers: initRecs}
				}
				err = initErr
			default:
				got, err = uc.CalculateForAll(context.Background(), "test-ns", "test-deployment", tt.container, "7d")
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("RecommenderUseCase.Calculate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if len(got.MainContainers) != len(tt.want.MainContainers) {
				t.Fatalf("got %d main recommendations, want %d", len(got.MainContainers), len(tt.want.MainContainers))
			}
			for i, gotRec := range got.MainContainers {
				wantRec := tt.want.MainContainers[i]
				if gotRec.ContainerName != wantRec.ContainerName {
					t.Errorf("MainRec[%d]: got container name %s, want %s", i, gotRec.ContainerName, wantRec.ContainerName)
				}
				if wantRec.Recommendation == nil {
					continue
				}
				assertRecommendation(t, fmt.Sprintf("MainRec[%d]", i), gotRec.Recommendation, wantRec.Recommendation)
			}
			if len(got.InitContainers) != len(tt.want.InitContainers) {
				t.Fatalf("got %d init recommendations, want %d", len(got.InitContainers), len(tt.want.InitContainers))
			}
			for i, gotRec := range got.InitContainers {
				wantRec := tt.want.InitContainers[i]
				if gotRec.ContainerName != wantRec.ContainerName {
					t.Errorf("InitRec[%d]: got container name %s, want %s", i, gotRec.ContainerName, wantRec.ContainerName)
				}
				if wantRec.Recommendation == nil {
					continue
				}
				assertRecommendation(t, fmt.Sprintf("InitRec[%d]", i), gotRec.Recommendation, wantRec.Recommendation)
			}
		})
	}
}

func assertRecommendation(t *testing.T, prefix string, got, want *entity.Recommendation) {
	if got.IsOOMKilled != want.IsOOMKilled {
		t.Errorf("%s: got IsOOMKilled %v, want %v", prefix, got.IsOOMKilled, want.IsOOMKilled)
	}
	if got.CPU.SpikinessWarning != want.CPU.SpikinessWarning {
		t.Errorf("%s: got SpikinessWarning %v, want %v", prefix, got.CPU.SpikinessWarning, want.CPU.SpikinessWarning)
	}
	if got.Memory.Cmp(*want.Memory) != 0 {
		t.Errorf("%s: got Memory %s, want %s", prefix, got.Memory.String(), want.Memory.String())
	}
	if got.CPU.Request.Cmp(*want.CPU.Request) != 0 {
		t.Errorf("%s: got CPU Request %s, want %s", prefix, got.CPU.Request.String(), want.CPU.Request.String())
	}
	if got.CPU.Limit.Cmp(*want.CPU.Limit) != 0 {
		t.Errorf("%s: got CPU Limit %s, want %s", prefix, got.CPU.Limit.String(), want.CPU.Limit.String())
	}
}
