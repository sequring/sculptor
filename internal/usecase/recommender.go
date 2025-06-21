package usecase

import (
	"context"
	"fmt"
	"log"

	"github.com/sequring/sculptor/internal/entity"
	"k8s.io/apimachinery/pkg/api/resource"
)

type RecommenderUseCase struct {
	k8sGateway  DeploymentGateway
	promGateway MetricsGateway
	silent     bool
}

func NewRecommenderUseCase(k8sGateway DeploymentGateway, promGateway MetricsGateway, silent bool) *RecommenderUseCase {
	return &RecommenderUseCase{
		k8sGateway:  k8sGateway,
		promGateway: promGateway,
		silent:     silent,
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
		if !uc.silent {
			log.Printf("No --container specified, automatically selected container: '%s'\n", targetContainerName)
		}
	}

	if !uc.silent {
		log.Println("Checking for OOMKilled events...")
	}
	isOOM, _, currentLimit, err := uc.k8sGateway.CheckForOOMKilledEvents(ctx, d, targetContainerName)
	if err != nil && !uc.silent {
		log.Printf("Warning: could not check for OOM events: %v", err)
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
		if !uc.silent {
			log.Println("No OOMKilled events found. Proceeding with Prometheus-based analysis for memory.")
		}
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
			if !uc.silent {
				log.Printf(
					"High CPU spikiness detected (P99/P50 ratio: %.2f > threshold: %.2f). Applying %.0f%% extra buffer to CPU limit.",
					spikinessRatio,
					spikinessThreshold,
					(spikinessCPUBuffer-1)*100,
				)
			}
		}
	}

	rec := &entity.Recommendation{
		Memory:      memRecommendation,
		IsOOMKilled: isOOMRecommendation,
		CPU: &entity.CPURecommendation{
			Request:          resource.NewMilliQuantity(int64(cpuP90*1000), resource.DecimalSI),
			Limit:            resource.NewMilliQuantity(int64(cpuP99*1000), resource.DecimalSI),
			SpikinessWarning: isSpiky,
		},
	}

	return rec, targetContainerName, nil
}
