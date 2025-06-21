package presenter

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/sequring/sculptor/internal/entity"
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
	logger *slog.Logger
}

func NewYAMLPresenter(logger *slog.Logger) *YAMLPresenter {
	return &YAMLPresenter{
		logger: logger,
	}
}

func (p *YAMLPresenter) Render(rec *entity.Recommendation, targetContainerName string) (string, error) {
	var warningHeader string
	if rec.IsOOMKilled {
		colorRed := "\033[31m"
		colorReset := "\033[0m"
		warningHeader = fmt.Sprintf("%s\n", colorRed)
		warningHeader += "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! WARNING !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!\n"
		warningHeader += "! OOMKilled event detected. Memory recommendation is aggressive.\n"
		warningHeader += "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!\n"
		warningHeader += fmt.Sprintf("%s", colorReset)
	}

	if rec.CPU.SpikinessWarning {
		colorYellow := "\033[33m"
		colorReset := "\033[0m"
		spikyWarning := fmt.Sprintf("%s", colorYellow)
		spikyWarning += "--- INFO: High CPU spikiness detected. An extra buffer has been added to the CPU limit. ---\n"
		spikyWarning += fmt.Sprintf("%s", colorReset)
		warningHeader += spikyWarning
	}

	if rec.IsOOMKilled || rec.CPU.SpikinessWarning {
		p.logger.Warn("Warning: OOMKilled or CPU spikiness detected. Review recommendations carefully.")
	}

	memString := formatMemoryHumanReadable(rec.Memory)
	prettyMem, err := resource.ParseQuantity(memString)
	if err != nil {
		return "", fmt.Errorf("internal error parsing pretty memory: %w", err)
	}

	prettyCPUReq, err := resource.ParseQuantity(rec.CPU.Request.String())
	if err != nil {
		return "", fmt.Errorf("internal error parsing pretty CPU request: %w", err)
	}

	prettyCPULim, err := resource.ParseQuantity(rec.CPU.Limit.String())
	if err != nil {
		return "", fmt.Errorf("internal error parsing pretty CPU limit: %w", err)
	}

	outputData := OutputYAML{
		Containers: []OutputContainer{
			{
				Name: targetContainerName,
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
			},
		},
	}

	yamlBytes, err := yaml.Marshal(outputData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal snippet to YAML: %w", err)
	}

	yamlHeader := "\n--- Recommended Resource Snippet (paste into your Deployment YAML) ---\n"

	finalOutput := ""
	if warningHeader != "" {
		finalOutput += warningHeader
	}
	finalOutput += yamlHeader
	finalOutput += string(yamlBytes)

	return finalOutput, nil
}

func formatMemoryHumanReadable(q *resource.Quantity) string {
	const (
		Ki = 1024
		Mi = 1024 * Ki
		Gi = 1024 * Mi
	)

	bytes := q.Value()
	if bytes == 0 {
		return "0"
	}

	switch {
	//case bytes >= Gi:
	//	return fmt.Sprintf("%.0fGi", math.Ceil(float64(bytes)/Gi))
	case bytes >= Mi:
		return fmt.Sprintf("%.0fMi", math.Ceil(float64(bytes)/Mi))
	case bytes >= Ki:
		return fmt.Sprintf("%.0fKi", math.Ceil(float64(bytes)/Ki))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func (p *YAMLPresenter) PrintLogo() {
	logo := `
  ██████  ▄████▄   █    ██  ██▓     ██▓███  ▄▄▄█████▓ ▒█████   ██▀███  
▒██    ▒ ▒██▀ ▀█   ██  ▓██▒▓██▒    ▓██░  ██▒▓  ██▒ ▓▒▒██▒  ██▒▓██ ▒ ██▒
░ ▓██▄   ▒▓█    ▄ ▓██  ▒██░▒██░    ▓██░ ██▓▒▒ ▓██░ ▒░▒██░  ██▒▓██ ░▄█ ▒
  ▒   ██▒▒▓▓▄ ▄██▒▓▓█  ░██░▒██░    ▒██▄█▓▒ ▒░ ▓██▓ ░ ▒██   ██░▒██▀▀█▄  
▒██████▒▒▒ ▓███▀ ░▒▒█████▓ ░██████▒▒██▒ ░  ░  ▒██▒ ░ ░ ████▓▒░░██▓ ▒██▒
▒ ▒▓▒ ▒ ░░ ░▒ ▒  ░░▒▓▒ ▒ ▒ ░ ▒░▓  ░▒▓▒░ ░  ░  ▒ ░░   ░ ▒░▒░▒░ ░ ▒▓ ░▒▓░
░ ░▒  ░ ░  ░  ▒   ░░▒░ ░ ░ ░ ░ ▒  ░░▒ ░         ░      ░ ▒ ▒░   ░▒ ░ ▒░
░  ░  ░  ░         ░░░ ░ ░   ░ ░   ░░         ░      ░ ░ ░ ▒    ░░   ░ 
      ░  ░ ░         ░         ░  ░                      ░ ░     ░     
         ░                                      
   Copyright © 2025 Valentyn Nastenko
`
	p.logger.Info(logo)
}
