package presenter

import (
	"fmt"
	"io"
	"math"
	"os"

	"github.com/sequring/sculptor/internal/usecase"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

type OutputYAML struct {
	Containers []OutputContainer `yaml:"containers"`
}

type OutputContainer struct {
	Name      string                  `yaml:"name"`
	Resources v1.ResourceRequirements `yaml:"resources"`
}

type YAMLPresenter struct {
	silent bool
	writer io.Writer
}

func NewYAMLPresenter(silent bool, writer io.Writer) *YAMLPresenter {
	return &YAMLPresenter{
		silent: silent,
		writer: writer,
	}
}

func (p *YAMLPresenter) Render(recs []usecase.NamedRecommendation) error {
	if len(recs) == 0 {
		if !p.silent {
			fmt.Fprintln(p.writer, "No recommendations could be generated. This may be due to a lack of metrics data.")
		}
		return nil
	}

	var allWarnings []string
	var outputContainers []OutputContainer

	for _, rec := range recs {
		if rec.Recommendation.IsOOMKilled {
			msg := fmt.Sprintf("OOMKilled event detected for container '%s'.", rec.ContainerName)
			allWarnings = append(allWarnings, msg)
		}
		if rec.Recommendation.CPU.SpikinessWarning {
			msg := fmt.Sprintf("High CPU spikiness detected for container '%s'.", rec.ContainerName)
			allWarnings = append(allWarnings, msg)
		}

		memString := formatMemoryHumanReadable(rec.Recommendation.Memory)
		prettyMem, err := resource.ParseQuantity(memString)
		if err != nil {
			return fmt.Errorf("internal error parsing memory for container %s: %w", rec.ContainerName, err)
		}

		prettyCPUReq, err := resource.ParseQuantity(rec.Recommendation.CPU.Request.String())
		if err != nil {
			return fmt.Errorf("internal error parsing CPU request for container %s: %w", rec.ContainerName, err)
		}

		prettyCPULim, err := resource.ParseQuantity(rec.Recommendation.CPU.Limit.String())
		if err != nil {
			return fmt.Errorf("internal error parsing CPU limit for container %s: %w", rec.ContainerName, err)
		}

		container := OutputContainer{
			Name: rec.ContainerName,
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU:    prettyCPUReq,
					v1.ResourceMemory: prettyMem,
				},
				Limits: v1.ResourceList{
					v1.ResourceCPU:    prettyCPULim,
					v1.ResourceMemory: prettyMem,
				},
			},
		}
		outputContainers = append(outputContainers, container)
	}

	if p.silent {
		if len(allWarnings) > 0 {
			fmt.Fprintln(os.Stderr, "Warning: OOMKilled events or CPU spikiness detected. Review recommendations carefully.")
		}
	} else {
		for _, warning := range allWarnings {
			colorYellow := "\033[33m"
			colorReset := "\033[0m"
			fmt.Fprintf(p.writer, "%s--- WARNING: %s ---%s\n", colorYellow, warning, colorReset)
		}
	}

	outputData := OutputYAML{
		Containers: outputContainers,
	}

	yamlBytes, err := yaml.Marshal(outputData)
	if err != nil {
		return fmt.Errorf("failed to marshal snippet to YAML: %w", err)
	}

	if !p.silent {
		fmt.Fprintln(p.writer, "\n--- Recommended Resource Snippet (paste into your Deployment YAML) ---")
	}

	_, err = p.writer.Write(yamlBytes)
	return err
}

func formatMemoryHumanReadable(q *resource.Quantity) string {
	const (
		Ki = 1024
		Mi = 1024 * Ki
		Gi = 1024 * Mi
	)

	if q == nil || q.IsZero() {
		return "0"
	}

	bytes := q.Value()
	switch {
	case bytes >= Mi:
		return fmt.Sprintf("%.0fMi", math.Ceil(float64(bytes)/Mi))
	case bytes >= Ki:
		return fmt.Sprintf("%.0fKi", math.Ceil(float64(bytes)/Ki))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
