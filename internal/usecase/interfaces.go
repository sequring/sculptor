package usecase

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// DeploymentParams represents reusable deployment arguments.
type DeploymentParams struct {
	Namespace       string
	DeploymentName  string
	TargetContainer string
	TimeRange       string
}

// AllRecommendations contains recommendations for both main and init containers
type AllRecommendations struct {
	MainContainers []NamedRecommendation
	InitContainers []NamedRecommendation
}

type DeploymentGateway interface {
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
	CheckForOOMKilledEvents(ctx context.Context, d *appsv1.Deployment, targetContainerName string) (bool, string, *resource.Quantity, error)
}

type MetricsGateway interface {
	GetMemoryMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetMemoryStdDevMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetCPURequestMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetCPULimitMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetCPUMedianMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetInitContainerMemoryMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
}

type Recommender interface {
	CalculateForDeployment(ctx context.Context, params DeploymentParams) ([]NamedRecommendation, error)
	CalculateForInitContainers(ctx context.Context, params DeploymentParams) ([]NamedRecommendation, error)
	CalculateForAll(ctx context.Context, namespace, deploymentName, targetContainerName, timeRange string) (*AllRecommendations, error)
}
