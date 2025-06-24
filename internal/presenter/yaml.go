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

type OutputContainer struct {
	Name      string                  `yaml:"name"`
	Resources v1.ResourceRequirements `yaml:"resources"`
}

type OutputYAML struct {
	Containers     []OutputContainer `yaml:"containers,omitempty"`
	InitContainers []OutputContainer `yaml:"initContainers,omitempty"`
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

func (p *YAMLPresenter) Render(recs *usecase.AllRecommendations) error {
	if recs == nil || (len(recs.MainContainers) == 0 && len(recs.InitContainers) == 0) {
		if !p.silent {
			fmt.Fprintln(p.writer, "No recommendations could be generated for any containers.")
		}
		return nil
	}

	mainContainers, mainWarnings, err := p.buildOutputContainers(recs.MainContainers)
	if err != nil {
		return err
	}
	initContainers, initWarnings, err := p.buildOutputContainers(recs.InitContainers)
	if err != nil {
		return err
	}

	allWarnings := append(mainWarnings, initWarnings...)
	p.printWarnings(allWarnings)

	outputData := make(map[string]interface{})

	if len(mainContainers) > 0 {
		outputData["containers"] = mainContainers
	}
	if len(initContainers) > 0 {
		outputData["initContainers"] = initContainers
	}

	yamlBytes, err := yaml.Marshal(outputData)
	if err != nil {
		return fmt.Errorf("failed to marshal combined YAML: %w", err)
	}

	p.printYAML(yamlBytes)
	return nil
}

func (p *YAMLPresenter) buildOutputContainers(recs []usecase.NamedRecommendation) ([]OutputContainer, []string, error) {
	var allWarnings []string
	var outputContainers []OutputContainer

	for _, rec := range recs {
		if rec.Recommendation.IsOOMKilled {
			allWarnings = append(allWarnings, fmt.Sprintf("OOMKilled event detected for container '%s'", rec.ContainerName))
		}
		if rec.Recommendation.CPU.SpikinessWarning {
			allWarnings = append(allWarnings, fmt.Sprintf("High CPU spikiness detected for container '%s'", rec.ContainerName))
		}

		memString := formatMemoryHumanReadable(rec.Recommendation.Memory)
		prettyMem, err := resource.ParseQuantity(memString)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing memory for %s: %w", rec.ContainerName, err)
		}
		prettyCPUReq, err := resource.ParseQuantity(rec.Recommendation.CPU.Request.String())
		if err != nil {
			return nil, nil, fmt.Errorf("parsing CPU request for %s: %w", rec.ContainerName, err)
		}
		prettyCPULim, err := resource.ParseQuantity(rec.Recommendation.CPU.Limit.String())
		if err != nil {
			return nil, nil, fmt.Errorf("parsing CPU limit for %s: %w", rec.ContainerName, err)
		}

		container := OutputContainer{
			Name: rec.ContainerName,
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceCPU: prettyCPUReq, v1.ResourceMemory: prettyMem},
				Limits:   v1.ResourceList{v1.ResourceCPU: prettyCPULim, v1.ResourceMemory: prettyMem},
			},
		}
		outputContainers = append(outputContainers, container)
	}
	return outputContainers, allWarnings, nil
}

func (p *YAMLPresenter) printWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	if p.silent {
		fmt.Fprintln(os.Stderr, "Warning: Issues like OOMKilled or CPU spikiness were detected. Review recommendations carefully.")
	} else {
		for _, warning := range warnings {
			fmt.Fprintf(p.writer, "\033[33m--- WARNING: %s ---\033[0m\n", warning)
		}
	}
}

func (p *YAMLPresenter) printYAML(yamlBytes []byte) {
	if !p.silent {
		fmt.Fprintln(p.writer, "\n--- Recommended Resource Snippet (paste into your Deployment YAML) ---")
	}
	p.writer.Write(yamlBytes)
}

func formatMemoryHumanReadable(q *resource.Quantity) string {
	const (
		Ki = 1024
		Mi = 1024 * Ki
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
