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
	Containers []OutputContainer `yaml:"containers,omitempty"`
}

type AllOutputYAML struct {
	Containers     []OutputContainer `yaml:"containers,omitempty"`
	InitContainers []OutputContainer `yaml:"initContainers,omitempty"`
}

type InitOutputYAML struct {
	InitContainers []OutputContainer `yaml:"initContainers,omitempty"`
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
			fmt.Fprintln(p.writer, "No recommendations could be generated for main containers.")
		}
		return nil
	}

	outputContainers, allWarnings := p.buildOutputContainers(recs)

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

	p.printYAML(yamlBytes, "main containers")
	return nil
}

func (p *YAMLPresenter) RenderInit(recs []usecase.NamedRecommendation) error {
	if len(recs) == 0 {
		if !p.silent {
			fmt.Fprintln(p.writer, "No recommendations could be generated for init containers.")
		}
		return nil
	}

	outputContainers, _ := p.buildOutputContainers(recs)

	outputData := InitOutputYAML{
		InitContainers: outputContainers,
	}

	yamlBytes, err := yaml.Marshal(outputData)
	if err != nil {
		return fmt.Errorf("failed to marshal init snippet to YAML: %w", err)
	}

	p.printYAML(yamlBytes, "init containers")
	return nil
}

func (p *YAMLPresenter) buildOutputContainers(recs []usecase.NamedRecommendation) ([]OutputContainer, []string) {
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
		prettyMem, _ := resource.ParseQuantity(memString)
		prettyCPUReq, _ := resource.ParseQuantity(rec.Recommendation.CPU.Request.String())
		prettyCPULim, _ := resource.ParseQuantity(rec.Recommendation.CPU.Limit.String())

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
	return outputContainers, allWarnings
}

// RenderAll renders recommendations for both main and init containers in a single YAML
func (p *YAMLPresenter) RenderAll(recs *usecase.AllRecommendations) error {
	if len(recs.MainContainers) == 0 && len(recs.InitContainers) == 0 {
		if !p.silent {
			fmt.Fprintln(p.writer, "No recommendations could be generated for any containers.")
		}
		return nil
	}

	// Process main containers
	mainContainers, mainWarnings := p.buildOutputContainers(recs.MainContainers)
	// Process init containers
	initContainers, initWarnings := p.buildOutputContainers(recs.InitContainers)

	allWarnings := append(mainWarnings, initWarnings...)

	// Print warnings if not in silent mode
	if !p.silent {
		for _, warning := range allWarnings {
			colorYellow := "\033[33m"
			colorReset := "\033[0m"
			fmt.Fprintf(p.writer, "%s--- WARNING: %s ---%s\n", colorYellow, warning, colorReset)
		}
	}

	outputData := AllOutputYAML{
		Containers:     mainContainers,
		InitContainers: initContainers,
	}

	yamlBytes, err := yaml.Marshal(outputData)
	if err != nil {
		return fmt.Errorf("failed to marshal combined YAML: %w", err)
	}

	p.printYAML(yamlBytes, "all containers")
	return nil
}

func (p *YAMLPresenter) printYAML(yamlBytes []byte, target string) {
	if !p.silent {
		header := fmt.Sprintf("\n--- Recommended Resource Snippet for %s (paste into your Deployment YAML) ---", target)
		fmt.Fprintln(p.writer, header)
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
