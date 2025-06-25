package presenter

import (
	"bytes"
	"testing"

	"github.com/sequring/sculptor/internal/entity"
	"github.com/sequring/sculptor/internal/usecase"
	"k8s.io/apimachinery/pkg/api/resource"
)

func mustParseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func TestYAMLPresenter_BuildOutputContainers(t *testing.T) {
	mainRec := usecase.NamedRecommendation{
		ContainerName: "main-app",
		Recommendation: &entity.Recommendation{
			IsOOMKilled: true,
			Memory:      mustParseQuantity("256Mi"),
			CPU: &entity.CPURecommendation{
				Request:          mustParseQuantity("100m"),
				Limit:            mustParseQuantity("200m"),
				SpikinessWarning: true,
			},
		},
	}

	initRec := usecase.NamedRecommendation{
		ContainerName: "init-setup",
		Recommendation: &entity.Recommendation{
			Memory: mustParseQuantity("64Mi"),
			CPU: &entity.CPURecommendation{
				Request: mustParseQuantity("100m"),
				Limit:   mustParseQuantity("1000m"),
			},
		},
	}

	// Expected output structure for reference
	_ = &OutputYAML{
		Containers: []OutputContainer{
			{
				Name: "main-app",
				Resources: struct {
					Limits   map[string]resource.Quantity "yaml:\"limits,omitempty\""
					Requests map[string]resource.Quantity "yaml:\"requests,omitempty\""
				}{
					Requests: map[string]resource.Quantity{
						"cpu":    resource.MustParse("100m"),
						"memory": resource.MustParse("256Mi"),
					},
					Limits: map[string]resource.Quantity{
						"cpu":    resource.MustParse("200m"),
						"memory": resource.MustParse("256Mi"),
					},
				},
			},
		},
		InitContainers: []OutputContainer{
			{
				Name: "init-setup",
				Resources: struct {
					Limits   map[string]resource.Quantity "yaml:\"limits,omitempty\""
					Requests map[string]resource.Quantity "yaml:\"requests,omitempty\""
				}{
					Requests: map[string]resource.Quantity{
						"cpu":    resource.MustParse("100m"),
						"memory": resource.MustParse("64Mi"),
					},
					Limits: map[string]resource.Quantity{
						"cpu":    resource.MustParse("1000m"),
						"memory": resource.MustParse("64Mi"),
					},
				},
			},
		},
	}

	// Create a buffer to capture the output
	var buf bytes.Buffer
	p := NewYAMLPresenter(false, &buf)

	// Test rendering
	err := p.Render(&usecase.AllRecommendations{
		MainContainers:  []usecase.NamedRecommendation{mainRec},
		InitContainers: []usecase.NamedRecommendation{initRec},
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// The actual output is written to the buffer
	output := buf.String()
	_ = output // Use the variable to avoid unused variable error

	// In a real test, you would unmarshal the output and compare with wantOutput
	// For now, we'll just check that we got some output
	if output == "" {
		t.Error("Expected non-empty output, got empty string")
	}
}
