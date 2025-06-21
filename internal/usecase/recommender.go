package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sequring/sculptor/internal/entity"
	"k8s.io/apimachinery/pkg/api/resource"
)

type RecommenderUseCase struct {
	k8sGateway  DeploymentGateway
	promGateway MetricsGateway
	logger      *slog.Logger
}

func NewRecommenderUseCase(k8sGateway DeploymentGateway, promGateway MetricsGateway, logger *slog.Logger) *RecommenderUseCase {
	return &RecommenderUseCase{
		k8sGateway:  k8sGateway,
		promGateway: promGateway,
		logger:      logger,
	}
}

const (
	spikinessThreshold  = 2.0
	spikinessCPUBuffer  = 1.25
	oomMemoryMultiplier = 1.5
)

func (uc *RecommenderUseCase) CalculateForDeployment(ctx context.Context, namespace, deploymentName, targetContainerName, timeRange string) (*entity.Recommendation, string, error) {
	d, err := uc.k8sGateway.GetDeployment(ctx, namespace, deploymentName)
	if err != nil {
		return nil, "", fmt.Errorf("could not get deployment: %w", err)
	}

	if targetContainerName == "" {
		found := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == deploymentName {
				targetContainerName = c.Name
				found = true
				break
			}
		}
		if !found {
			for _, c := range d.Spec.Template.Spec.Containers {
				if c.Name == "app" {
					targetContainerName = c.Name
					found = true
					break
				}
			}
		}
		if !found {
			if len(d.Spec.Template.Spec.Containers) == 0 {
				return nil, "", fmt.Errorf("no containers found in deployment spec")
			}
			targetContainerName = d.Spec.Template.Spec.Containers[len(d.Spec.Template.Spec.Containers)-1].Name
		}
		uc.logger.Info("No --container specified, automatically selected container", "container", targetContainerName)
	}

	uc.logger.Info("Checking for OOMKilled events...")
	isOOM, _, currentLimit, err := uc.k8sGateway.CheckForOOMKilledEvents(ctx, d, targetContainerName)
	if err != nil {
		uc.logger.Warn("Could not check for OOM events", "error", err)
	}

	var memRecommendation *resource.Quantity
	isOOMRecommendation := false

	if isOOM {
		isOOMRecommendation = true
		if currentLimit != nil {
			newVal := int64(float64(currentLimit.Value()) * oomMemoryMultiplier)
			memRecommendation = resource.NewQuantity(newVal, resource.BinarySI)
		} else {
			memRecommendation = resource.NewQuantity(1024*1024*1024, resource.BinarySI)
		}
	} else {
		uc.logger.Info("No OOMKilled events found. Proceeding with Prometheus-based analysis for memory.")
		memP99, err := uc.promGateway.GetMemoryMetrics(ctx, namespace, deploymentName, timeRange)
		if err != nil {
			return nil, "", err
		}
		memBytes := memP99 * 1.2
		memRecommendation = resource.NewQuantity(int64(memBytes), resource.BinarySI)
	}

	cpuP90, err := uc.promGateway.GetCPURequestMetrics(ctx, namespace, deploymentName, timeRange)
	if err != nil {
		return nil, "", err
	}
	cpuP99, err := uc.promGateway.GetCPULimitMetrics(ctx, namespace, deploymentName, timeRange)
	if err != nil {
		return nil, "", err
	}
	cpuP50, err := uc.promGateway.GetCPUMedianMetrics(ctx, namespace, deploymentName, timeRange)
	if err != nil {
		return nil, "", err
	}

	cpuLimitValue := cpuP99
	isSpiky := false

	if cpuP50 > 0 {
		spikinessRatio := cpuP99 / cpuP50
		if spikinessRatio > spikinessThreshold {
			isSpiky = true
			cpuLimitValue *= spikinessCPUBuffer
			uc.logger.Info("High CPU spikiness detected",
				"p99_p50_ratio", spikinessRatio,
				"threshold", spikinessThreshold,
				"extra_buffer_percent", (spikinessCPUBuffer-1)*100,
				"message", "Applying extra buffer to CPU limit")
		}
	}

	rec := &entity.Recommendation{
		Memory:      memRecommendation,
		IsOOMKilled: isOOMRecommendation,
		CPU: &entity.CPURecommendation{
			Request: resource.NewMilliQuantity(int64(cpuP90*1000), resource.DecimalSI),
			Limit:   resource.NewMilliQuantity(int64(cpuLimitValue*1000), resource.DecimalSI),

			SpikinessWarning: isSpiky,
		},
	}

	return rec, targetContainerName, nil
}
