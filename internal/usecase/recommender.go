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
	spikinessThreshold               = 2.0
	spikinessCPUBuffer               = 1.25
	oomMemoryMultiplier              = 1.5
	mainContainerMemoryBufferPercent = 120 // Represents 1.2x buffer
	initContainerMemoryBufferPercent = 115 // Represents 1.15x buffer
	initMemoryDefault                = "128Mi"
	initCPURequestDefault            = "100m"
	initCPULimitDefault              = "1000m"
	minCPURequestMilli               = 50
	minCPULimitMilli                 = 100
	minMemoryBytes                   = 64 * 1024 * 1024
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
		found := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == targetContainerName {
				found = true
				break
			}
		}
		if found {
			containersToAnalyze = append(containersToAnalyze, targetContainerName)
		} else {
			return []NamedRecommendation{}, nil
		}
	} else {
		for _, c := range d.Spec.Template.Spec.Containers {
			containersToAnalyze = append(containersToAnalyze, c.Name)
		}
	}

	if len(containersToAnalyze) == 0 {
		return []NamedRecommendation{}, nil
	}

	var finalRecommendations []NamedRecommendation
	for _, containerName := range containersToAnalyze {
		isOOM, _, currentLimit, _ := uc.k8sGateway.CheckForOOMKilledEvents(ctx, d, containerName)

		var memRecommendation *resource.Quantity
		isOOMRecommendation := false
		if isOOM {
			isOOMRecommendation = true
			if currentLimit != nil {
				newVal := int64(float64(currentLimit.Value()) * oomMemoryMultiplier)
				memRecommendation = resource.NewQuantity(newVal, resource.BinarySI)
			} else {
				memRecommendation = resource.NewQuantity(1024*1024*512, resource.BinarySI)
			}
		} else {
			memP99, _ := uc.promGateway.GetMemoryMetrics(ctx, namespace, deploymentName, containerName, timeRange)
			memBytes := (int64(memP99) * mainContainerMemoryBufferPercent) / 100
			memRecommendation = resource.NewQuantity(memBytes, resource.BinarySI)
		}

		cpuP90, _ := uc.promGateway.GetCPURequestMetrics(ctx, namespace, deploymentName, containerName, timeRange)
		cpuP99, _ := uc.promGateway.GetCPULimitMetrics(ctx, namespace, deploymentName, containerName, timeRange)
		cpuP50, _ := uc.promGateway.GetCPUMedianMetrics(ctx, namespace, deploymentName, containerName, timeRange)

		cpuLimitValue := cpuP99
		isSpiky := false
		if cpuP50 > 0 && cpuP99/cpuP50 > spikinessThreshold {
			isSpiky = true
			cpuLimitValue *= spikinessCPUBuffer
		}

		calculatedCPURequestMilli := int64(cpuP90 * 1000)
		if calculatedCPURequestMilli < minCPURequestMilli {
			uc.logger.Info(
				"Calculated CPU request is below the minimum floor, applying minimum.",
				"container", containerName,
				"calculated", fmt.Sprintf("%dm", calculatedCPURequestMilli),
				"minimum", fmt.Sprintf("%dm", minCPURequestMilli),
			)
			calculatedCPURequestMilli = minCPURequestMilli
		}

		calculatedCPULimitMilli := int64(cpuLimitValue * 1000)
		if calculatedCPULimitMilli < minCPULimitMilli {
			uc.logger.Info(
				"Calculated CPU limit is below the minimum floor, applying minimum.",
				"container", containerName,
				"calculated", fmt.Sprintf("%dm", calculatedCPULimitMilli),
				"minimum", fmt.Sprintf("%dm", minCPULimitMilli),
			)
			calculatedCPULimitMilli = minCPULimitMilli
		}

		if calculatedCPULimitMilli < calculatedCPURequestMilli {
			uc.logger.Info(
				"Calculated CPU limit is below the CPU request, applying CPU request as limit.",
				"container", containerName,
				"calculated", fmt.Sprintf("%dm", calculatedCPULimitMilli),
				"request", fmt.Sprintf("%dm", calculatedCPURequestMilli),
			)
			calculatedCPULimitMilli = calculatedCPURequestMilli
		}

		calculatedMemoryBytes := memRecommendation.Value()
		if calculatedMemoryBytes < minMemoryBytes {
			uc.logger.Info(
				"Calculated memory is below the minimum floor, applying minimum.",
				"container", containerName,
				"calculated", fmt.Sprintf("%dMi", calculatedMemoryBytes),
				"minimum", fmt.Sprintf("%dMi", minMemoryBytes),
			)
			calculatedMemoryBytes = minMemoryBytes
		}

		memRecommendation = resource.NewQuantity(calculatedMemoryBytes, resource.BinarySI)
		cpuRequest := resource.NewMilliQuantity(calculatedCPURequestMilli, resource.DecimalSI)
		cpuLimit := resource.NewMilliQuantity(calculatedCPULimitMilli, resource.DecimalSI)

		rec := &entity.Recommendation{
			Memory:      memRecommendation,
			IsOOMKilled: isOOMRecommendation,
			CPU: &entity.CPURecommendation{
				Request:          cpuRequest,
				Limit:            cpuLimit,
				SpikinessWarning: isSpiky,
			},
		}
		finalRecommendations = append(finalRecommendations, NamedRecommendation{ContainerName: containerName, Recommendation: rec})
	}
	return finalRecommendations, nil
}

func (uc *RecommenderUseCase) CalculateForInitContainers(ctx context.Context, namespace, deploymentName, targetContainerName, timeRange string) ([]NamedRecommendation, error) {
	d, err := uc.k8sGateway.GetDeployment(ctx, namespace, deploymentName)
	if err != nil {
		return nil, fmt.Errorf("could not get deployment: %w", err)
	}

	var containersToAnalyze []string
	if targetContainerName != "" {
		found := false
		for _, c := range d.Spec.Template.Spec.InitContainers {
			if c.Name == targetContainerName {
				found = true
				break
			}
		}
		if found {
			containersToAnalyze = append(containersToAnalyze, targetContainerName)
		} else {
			return []NamedRecommendation{}, nil
		}
	} else {
		for _, c := range d.Spec.Template.Spec.InitContainers {
			containersToAnalyze = append(containersToAnalyze, c.Name)
		}
	}

	if len(containersToAnalyze) == 0 {
		return []NamedRecommendation{}, nil
	}

	var finalRecommendations []NamedRecommendation
	for _, containerName := range containersToAnalyze {
		var memRecommendation *resource.Quantity
		memMax, _ := uc.promGateway.GetInitContainerMemoryMetrics(ctx, namespace, deploymentName, containerName, timeRange)

		if memMax > 0 {
			// FIX: Use integer math to avoid float inaccuracies
			memBytes := (int64(memMax) * initContainerMemoryBufferPercent) / 100
			memRecommendation = resource.NewQuantity(memBytes, resource.BinarySI)
		} else {
			memRecommendationVal := resource.MustParse(initMemoryDefault)
			memRecommendation = &memRecommendationVal
		}
		cpuRequest := resource.MustParse(initCPURequestDefault)
		cpuLimit := resource.MustParse(initCPULimitDefault)
		rec := &entity.Recommendation{
			Memory:      memRecommendation,
			IsOOMKilled: false,
			CPU: &entity.CPURecommendation{
				Request:          &cpuRequest,
				Limit:            &cpuLimit,
				SpikinessWarning: false,
			},
		}
		finalRecommendations = append(finalRecommendations, NamedRecommendation{ContainerName: containerName, Recommendation: rec})
	}
	return finalRecommendations, nil
}

func (uc *RecommenderUseCase) CalculateForAll(ctx context.Context, namespace, deploymentName, targetContainerName, timeRange string) (*AllRecommendations, error) {
	mainRecs, err := uc.CalculateForDeployment(ctx, namespace, deploymentName, targetContainerName, timeRange)
	if err != nil {
		return nil, fmt.Errorf("error calculating main container recommendations: %w", err)
	}
	initRecs, err := uc.CalculateForInitContainers(ctx, namespace, deploymentName, targetContainerName, timeRange)
	if err != nil {
		return nil, fmt.Errorf("error calculating init container recommendations: %w", err)
	}
	return &AllRecommendations{
		MainContainers: mainRecs,
		InitContainers: initRecs,
	}, nil
}
