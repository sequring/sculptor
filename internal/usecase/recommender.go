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

type NamedRecommendation struct {
	ContainerName  string
	Recommendation *entity.Recommendation
}

func (uc *RecommenderUseCase) CalculateForDeployment(ctx context.Context, namespace, deploymentName, targetContainerName, timeRange string) ([]NamedRecommendation, error) {
	d, err := uc.k8sGateway.GetDeployment(ctx, namespace, deploymentName)
	if err != nil {
		return nil, fmt.Errorf("could not get deployment: %w", err)
	}

	var containersToAnalyze []string
	if targetContainerName != "" {
		containersToAnalyze = append(containersToAnalyze, targetContainerName)
		uc.logger.Info("Analyzing specified container", "container", targetContainerName)
	} else {
		for _, c := range d.Spec.Template.Spec.Containers {
			containersToAnalyze = append(containersToAnalyze, c.Name)
		}
		uc.logger.Info("No container specified, analyzing all containers in the deployment", "containers", containersToAnalyze)
	}

	if len(containersToAnalyze) == 0 {
		return nil, fmt.Errorf("no containers found to analyze in deployment spec")
	}

	var finalRecommendations []NamedRecommendation

	for _, containerName := range containersToAnalyze {
		uc.logger.Info("Analyzing container", "container", containerName)

		isOOM, _, currentLimit, err := uc.k8sGateway.CheckForOOMKilledEvents(ctx, d, containerName)
		if err != nil {
			uc.logger.Warn("Could not check for OOM events", "container", containerName, "error", err)
		}

		var memRecommendation *resource.Quantity
		isOOMRecommendation := false

		if isOOM {
			isOOMRecommendation = true
			uc.logger.Warn("OOMKilled event detected for container", "container", containerName)
			if currentLimit != nil {
				newVal := int64(float64(currentLimit.Value()) * oomMemoryMultiplier)
				memRecommendation = resource.NewQuantity(newVal, resource.BinarySI)
			} else {
				memRecommendation = resource.NewQuantity(1024*1024*512, resource.BinarySI) // 512Mi fallback
			}
		} else {
			memP99, err := uc.promGateway.GetMemoryMetrics(ctx, namespace, deploymentName, containerName, timeRange)
			if err != nil {
				return nil, err
			}
			memBytes := memP99 * 1.2
			memRecommendation = resource.NewQuantity(int64(memBytes), resource.BinarySI)
		}

		cpuP90, err := uc.promGateway.GetCPURequestMetrics(ctx, namespace, deploymentName, containerName, timeRange)
		if err != nil {
			return nil, err
		}
		cpuP99, err := uc.promGateway.GetCPULimitMetrics(ctx, namespace, deploymentName, containerName, timeRange)
		if err != nil {
			return nil, err
		}
		cpuP50, err := uc.promGateway.GetCPUMedianMetrics(ctx, namespace, deploymentName, containerName, timeRange)
		if err != nil {
			return nil, err
		}

		cpuLimitValue := cpuP99
		isSpiky := false

		if cpuP50 > 0 && cpuP99/cpuP50 > spikinessThreshold {
			isSpiky = true
			cpuLimitValue *= spikinessCPUBuffer
			uc.logger.Info("High CPU spikiness detected",
				"container", containerName,
				"p99_p50_ratio", fmt.Sprintf("%.2f", cpuP99/cpuP50),
				"threshold", spikinessThreshold,
			)
		}

		if cpuP90 == 0 && cpuP99 == 0 && memRecommendation.Value() == 0 {
			uc.logger.Info("No usage data found for container, skipping recommendation", "container", containerName)
			continue
		}

		rec := &entity.Recommendation{
			Memory:      memRecommendation,
			IsOOMKilled: isOOMRecommendation,
			CPU: &entity.CPURecommendation{
				Request:          resource.NewMilliQuantity(int64(cpuP90*1000), resource.DecimalSI),
				Limit:            resource.NewMilliQuantity(int64(cpuLimitValue*1000), resource.DecimalSI),
				SpikinessWarning: isSpiky,
			},
		}

		finalRecommendations = append(finalRecommendations, NamedRecommendation{
			ContainerName:  containerName,
			Recommendation: rec,
		})
	}

	return finalRecommendations, nil
}
