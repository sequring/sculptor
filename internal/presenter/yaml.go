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
	Containers     []OutputContainer `yaml:"containers,omitempty"`
	InitContainers []OutputContainer `yaml:"initContainers,omitempty"`
}

// MarshalYAML implements custom YAML marshaling for OutputYAML
func (o OutputYAML) MarshalYAML() (interface{}, error) {
	type Alias OutputYAML
	return &struct {
		Containers     []OutputContainer `yaml:"containers,omitempty"`
		InitContainers []OutputContainer `yaml:"initContainers,omitempty"`
	}{
		Containers:     o.Containers,
		InitContainers: o.InitContainers,
	}, nil
}

type OutputContainer struct {
	Name      string `yaml:"name"`
	Resources ContainerResources `yaml:"resources"`
}

type ContainerResources struct {
	Limits   map[string]resource.Quantity `yaml:"limits,omitempty"`
	Requests map[string]resource.Quantity `yaml:"requests,omitempty"`
}

// MarshalYAML implements custom YAML marshaling for OutputContainer
func (c OutputContainer) MarshalYAML() (interface{}, error) {
	type Alias OutputContainer
	return &struct {
		Name      string `yaml:"name"`
		Resources ContainerResources `yaml:"resources"`
	}{
		Name:      c.Name,
		Resources: c.Resources,
	}, nil
}

// NewOutputYAML creates a properly formatted YAML output structure
func NewOutputYAML() *OutputYAML {
	return &OutputYAML{
		Containers:     []OutputContainer{},
		InitContainers: []OutputContainer{},
	}
}

// AddContainer adds a container to the output with properly formatted resources
func (o *OutputYAML) AddContainer(name string, isInit bool, requests, limits v1.ResourceList) {
	container := OutputContainer{
		Name: name,
		Resources: ContainerResources{
			Requests: make(map[string]resource.Quantity),
			Limits:   make(map[string]resource.Quantity),
		},
	}

	// Convert resource lists to the format we want
	for k, v := range requests {
		container.Resources.Requests[string(k)] = v.DeepCopy()
	}

	for k, v := range limits {
		container.Resources.Limits[string(k)] = v.DeepCopy()
	}

	if isInit {
		o.InitContainers = append(o.InitContainers, container)
	} else {
		o.Containers = append(o.Containers, container)
	}
}

// ToYAML converts the OutputYAML to a YAML string with proper formatting
func (o *OutputYAML) ToYAML() ([]byte, error) {
	output := make(map[string]interface{})

	// Convert containers
	if len(o.Containers) > 0 {
		containers := make([]map[string]interface{}, 0, len(o.Containers))
		for _, c := range o.Containers {
			container := map[string]interface{}{
				"name": c.Name,
				"resources": map[string]interface{}{
					"limits":   c.Resources.Limits,
					"requests": c.Resources.Requests,
				},
			}
			containers = append(containers, container)
		}
		output["containers"] = containers
	}

	// Convert init containers
	if len(o.InitContainers) > 0 {
		initContainers := make([]map[string]interface{}, 0, len(o.InitContainers))
		for _, c := range o.InitContainers {
			container := map[string]interface{}{
				"name": c.Name,
				"resources": map[string]interface{}{
					"limits":   c.Resources.Limits,
					"requests": c.Resources.Requests,
				},
			}
			initContainers = append(initContainers, container)
		}
		output["initContainers"] = initContainers
	}

	return yaml.Marshal(output)
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

	output := NewOutputYAML()
	var allWarnings []string

	// Process main containers
	for _, rec := range recs.MainContainers {
		if rec.Recommendation == nil {
			continue
		}

		if rec.Recommendation.IsOOMKilled {
			allWarnings = append(allWarnings, fmt.Sprintf("OOMKilled event detected for container '%s'", rec.ContainerName))
		}
		if rec.Recommendation.CPU.SpikinessWarning {
			allWarnings = append(allWarnings, fmt.Sprintf("High CPU spikiness detected for container '%s'", rec.ContainerName))
		}

		memString := formatMemoryHumanReadable(rec.Recommendation.Memory)
		prettyMem, err := resource.ParseQuantity(memString)
		if err != nil {
			return fmt.Errorf("parsing memory for %s: %w", rec.ContainerName, err)
		}

		requests := v1.ResourceList{
			v1.ResourceCPU:    *rec.Recommendation.CPU.Request,
			v1.ResourceMemory: prettyMem,
		}

		limits := v1.ResourceList{
			v1.ResourceCPU:    *rec.Recommendation.CPU.Limit,
			v1.ResourceMemory: prettyMem,
		}

		output.AddContainer(rec.ContainerName, false, requests, limits)
	}

	// Process init containers
	for _, rec := range recs.InitContainers {
		if rec.Recommendation == nil {
			continue
		}

		memString := formatMemoryHumanReadable(rec.Recommendation.Memory)
		prettyMem, err := resource.ParseQuantity(memString)
		if err != nil {
			return fmt.Errorf("parsing memory for init container %s: %w", rec.ContainerName, err)
		}

		requests := v1.ResourceList{
			v1.ResourceCPU:    *rec.Recommendation.CPU.Request,
			v1.ResourceMemory: prettyMem,
		}

		limits := v1.ResourceList{
			v1.ResourceCPU:    *rec.Recommendation.CPU.Limit,
			v1.ResourceMemory: prettyMem,
		}

		output.AddContainer(rec.ContainerName, true, requests, limits)
	}

	p.printWarnings(allWarnings)

	yamlBytes, err := output.ToYAML()
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	p.printYAML(yamlBytes)
	return nil
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
		KiB = 1024
		MiB = 1024 * KiB
	)

	if q == nil || q.IsZero() {
		return "0"
	}

	val := float64(q.Value())

	switch {
	case val >= MiB:
		return fmt.Sprintf("%.0fMi", math.Ceil(val/MiB))
	case val >= KiB:
		return fmt.Sprintf("%.0fKi", math.Ceil(val/KiB))
	default:
		return fmt.Sprintf("%dB", int64(val))
	}
}
