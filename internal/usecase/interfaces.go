package usecase

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type DeploymentGateway interface {
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
	CheckForOOMKilledEvents(ctx context.Context, d *appsv1.Deployment, targetContainerName string) (isOOMKilled bool, podName string, currentLimit *resource.Quantity, err error)
}

type MetricsGateway interface {
	GetMemoryMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetCPURequestMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetCPULimitMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
	GetCPUMedianMetrics(ctx context.Context, namespace, deploymentName, containerName, timeRange string) (float64, error)
}
