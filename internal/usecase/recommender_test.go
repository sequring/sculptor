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

// --- Test Suite ---

func TestRecommenderUseCase_CalculateForDeployment_HappyPath(t *testing.T) {
	// Arrange
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "main-app"}},
				},
			},
		},
	}

	deploymentGW := &mockDeploymentGateway{deployment: baseDeployment}
	metricsGW := &mockMetricsGateway{
		memValue:    100 * 1024 * 1024,
		cpuP90Value: 0.2,
		cpuP99Value: 0.4,
		cpuP50Value: 0.25,
	}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForDeployment(context.Background(), params)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recommendations))
	}

	rec := recommendations[0]
	if rec.ContainerName != "main-app" {
		t.Errorf("expected container name 'main-app', got '%s'", rec.ContainerName)
	}

	wantMemory := quantityFromInt((100 * 1024 * 1024 * mainContainerMemoryBufferPercent) / 100)
	if rec.Recommendation.Memory.Cmp(*wantMemory) != 0 {
		t.Errorf("Memory: got %s, want %s", rec.Recommendation.Memory.String(), wantMemory.String())
	}

	wantCPURequest := mustParseQuantity("200m")
	if rec.Recommendation.CPU.Request.Cmp(*wantCPURequest) != 0 {
		t.Errorf("CPU Request: got %s, want %s", rec.Recommendation.CPU.Request.String(), wantCPURequest.String())
	}

	wantCPULimit := mustParseQuantity("400m")
	if rec.Recommendation.CPU.Limit.Cmp(*wantCPULimit) != 0 {
		t.Errorf("CPU Limit: got %s, want %s", rec.Recommendation.CPU.Limit.String(), wantCPULimit.String())
	}
}

func TestRecommenderUseCase_CalculateForDeployment_OOMKilled(t *testing.T) {
	// Arrange
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "main-app"}},
				},
			},
		},
	}

	deploymentGW := &mockDeploymentGateway{
		deployment:      baseDeployment,
		isOOMKilled:     true,
		oomPodName:      "main-app",
		oomCurrentLimit: mustParseQuantity("256Mi"),
	}
	metricsGW := &mockMetricsGateway{}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForDeployment(context.Background(), params)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recommendations))
	}

	rec := recommendations[0]
	if !rec.Recommendation.IsOOMKilled {
		t.Error("expected IsOOMKilled to be true")
	}

	wantMemory := mustParseQuantity(fmt.Sprintf("%dMi", int(256*oomMemoryMultiplier)))
	if rec.Recommendation.Memory.Cmp(*wantMemory) != 0 {
		t.Errorf("Memory: got %s, want %s", rec.Recommendation.Memory.String(), wantMemory.String())
	}
}

func TestRecommenderUseCase_CalculateForDeployment_CPUSpikiness(t *testing.T) {
	// Arrange
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "main-app"}},
				},
			},
		},
	}

	deploymentGW := &mockDeploymentGateway{deployment: baseDeployment}
	metricsGW := &mockMetricsGateway{
		memValue:    80 * 1024 * 1024, // 80Mi (> 64Mi floor)
		cpuP90Value: 0.1,
		cpuP99Value: 0.5,
		cpuP50Value: 0.1,
	}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForDeployment(context.Background(), params)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recommendations))
	}

	rec := recommendations[0]
	if !rec.Recommendation.CPU.SpikinessWarning {
		t.Error("expected SpikinessWarning to be true")
	}

	wantLimit := mustParseQuantity(fmt.Sprintf("%dm", int(0.5*spikinessCPUBuffer*1000)))
	if rec.Recommendation.CPU.Limit.Cmp(*wantLimit) != 0 {
		t.Errorf("CPU Limit: got %s, want %s", rec.Recommendation.CPU.Limit.String(), wantLimit.String())
	}
}

func TestRecommenderUseCase_CalculateForDeployment_VeryLowUsage(t *testing.T) {
	// Arrange
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "main-app"}},
				},
			},
		},
	}

	deploymentGW := &mockDeploymentGateway{deployment: baseDeployment}
	metricsGW := &mockMetricsGateway{
		memValue:    10 * 1024 * 1024,
		cpuP90Value: 0.01,
		cpuP99Value: 0.02,
	}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForDeployment(context.Background(), params)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recommendations))
	}

	rec := recommendations[0]
	wantMemory := quantityFromInt(minMemoryBytes)
	if rec.Recommendation.Memory.Cmp(*wantMemory) != 0 {
		t.Errorf("Memory: got %s, want %s", rec.Recommendation.Memory.String(), wantMemory.String())
	}

	wantCPURequest := mustParseQuantity(fmt.Sprintf("%dm", minCPURequestMilli))
	if rec.Recommendation.CPU.Request.Cmp(*wantCPURequest) != 0 {
		t.Errorf("CPU Request: got %s, want %s", rec.Recommendation.CPU.Request.String(), wantCPURequest.String())
	}

	wantCPULimit := mustParseQuantity(fmt.Sprintf("%dm", minCPULimitMilli))
	if rec.Recommendation.CPU.Limit.Cmp(*wantCPULimit) != 0 {
		t.Errorf("CPU Limit: got %s, want %s", rec.Recommendation.CPU.Limit.String(), wantCPULimit.String())
	}
}

func TestRecommenderUseCase_CalculateForDeployment_NoMetricsData(t *testing.T) {
	// Arrange
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "main-app"}},
				},
			},
		},
	}

	deploymentGW := &mockDeploymentGateway{deployment: baseDeployment}
	metricsGW := &mockMetricsGateway{}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForDeployment(context.Background(), params)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recommendations))
	}

	rec := recommendations[0]
	wantMemory := quantityFromInt(minMemoryBytes)
	if rec.Recommendation.Memory.Cmp(*wantMemory) != 0 {
		t.Errorf("Memory: got %s, want %s", rec.Recommendation.Memory.String(), wantMemory.String())
	}

	wantCPURequest := mustParseQuantity(fmt.Sprintf("%dm", minCPURequestMilli))
	if rec.Recommendation.CPU.Request.Cmp(*wantCPURequest) != 0 {
		t.Errorf("CPU Request: got %s, want %s", rec.Recommendation.CPU.Request.String(), wantCPURequest.String())
	}

	wantCPULimit := mustParseQuantity(fmt.Sprintf("%dm", minCPULimitMilli))
	if rec.Recommendation.CPU.Limit.Cmp(*wantCPULimit) != 0 {
		t.Errorf("CPU Limit: got %s, want %s", rec.Recommendation.CPU.Limit.String(), wantCPULimit.String())
	}
}

func TestRecommenderUseCase_CalculateForInitContainers_HappyPath(t *testing.T) {
	// Arrange
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{{Name: "init-setup"}},
				},
			},
		},
	}

	deploymentGW := &mockDeploymentGateway{deployment: baseDeployment}
	metricsGW := &mockMetricsGateway{
		initMemValue: 50 * 1024 * 1024,
	}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForInitContainers(context.Background(), params)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recommendations))
	}

	rec := recommendations[0]
	if rec.ContainerName != "init-setup" {
		t.Errorf("expected container name 'init-setup', got '%s'", rec.ContainerName)
	}

	wantMemory := quantityFromInt((50 * 1024 * 1024 * initContainerMemoryBufferPercent) / 100)
	if rec.Recommendation.Memory.Cmp(*wantMemory) != 0 {
		t.Errorf("Memory: got %s, want %s", rec.Recommendation.Memory.String(), wantMemory.String())
	}

	wantCPURequest := mustParseQuantity(initCPURequestDefault)
	if rec.Recommendation.CPU.Request.Cmp(*wantCPURequest) != 0 {
		t.Errorf("CPU Request: got %s, want %s", rec.Recommendation.CPU.Request.String(), wantCPURequest.String())
	}

	wantCPULimit := mustParseQuantity(initCPULimitDefault)
	if rec.Recommendation.CPU.Limit.Cmp(*wantCPULimit) != 0 {
		t.Errorf("CPU Limit: got %s, want %s", rec.Recommendation.CPU.Limit.String(), wantCPULimit.String())
	}
}

func TestRecommenderUseCase_CalculateForInitContainers_NoMetricsData(t *testing.T) {
	// Arrange
	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-ns"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{{Name: "init-setup"}},
				},
			},
		},
	}

	deploymentGW := &mockDeploymentGateway{deployment: baseDeployment}
	metricsGW := &mockMetricsGateway{}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForInitContainers(context.Background(), params)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recommendations))
	}

	rec := recommendations[0]
	wantMemory := resource.MustParse(initMemoryDefault)
	if rec.Recommendation.Memory.Cmp(wantMemory) != 0 {
		t.Errorf("Memory: got %s, want %s", rec.Recommendation.Memory.String(), wantMemory.String())
	}
}

func TestRecommenderUseCase_CalculateForAll_HappyPath(t *testing.T) {
	// Arrange
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

	deploymentGW := &mockDeploymentGateway{deployment: baseDeployment}
	metricsGW := &mockMetricsGateway{
		memValue:     100 * 1024 * 1024,
		cpuP90Value:  0.2,
		cpuP99Value:  0.4,
		cpuP50Value:  0.25,
		initMemValue: 50 * 1024 * 1024,
	}
	uc := NewRecommenderUseCase(deploymentGW, metricsGW, newTestLogger())

	params := DeploymentParams{
		Namespace:      "test-ns",
		DeploymentName: "test-deployment",
		TimeRange:      "7d",
	}

	// Act
	recommendations, err := uc.CalculateForAll(context.Background(), params.Namespace, params.DeploymentName, params.TargetContainer, params.TimeRange)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recommendations.MainContainers) != 1 {
		t.Fatalf("expected 1 main recommendation, got %d", len(recommendations.MainContainers))
	}
	if len(recommendations.InitContainers) != 1 {
		t.Fatalf("expected 1 init recommendation, got %d", len(recommendations.InitContainers))
	}

	mainRec := recommendations.MainContainers[0]
	if mainRec.ContainerName != "main-app" {
		t.Errorf("expected main container name 'main-app', got '%s'", mainRec.ContainerName)
	}

	initRec := recommendations.InitContainers[0]
	if initRec.ContainerName != "init-setup" {
		t.Errorf("expected init container name 'init-setup', got '%s'", initRec.ContainerName)
	}

	// Assert main container recommendation
	wantMainMemory := quantityFromInt((100 * 1024 * 1024 * mainContainerMemoryBufferPercent) / 100)
	wantMainCPURequest := mustParseQuantity("200m")
	wantMainCPULimit := mustParseQuantity("400m")
	assertRecommendation(t, "MainContainer", mainRec.Recommendation, &entity.Recommendation{
		Memory:      wantMainMemory,
		IsOOMKilled: false,
		CPU: &entity.CPURecommendation{
			Request:          wantMainCPURequest,
			Limit:            wantMainCPULimit,
			SpikinessWarning: false,
		},
	})

	// Assert init container recommendation
	wantInitMemory := quantityFromInt((50 * 1024 * 1024 * initContainerMemoryBufferPercent) / 100)
	wantInitCPURequest := mustParseQuantity(initCPURequestDefault)
	wantInitCPULimit := mustParseQuantity(initCPULimitDefault)
	assertRecommendation(t, "InitContainer", initRec.Recommendation, &entity.Recommendation{
		Memory:      wantInitMemory,
		IsOOMKilled: false,
		CPU: &entity.CPURecommendation{
			Request:          wantInitCPURequest,
			Limit:            wantInitCPULimit,
			SpikinessWarning: false,
		},
	})
}