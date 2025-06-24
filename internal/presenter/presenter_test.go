package presenter

import (
	"testing"

	"github.com/sequring/sculptor/internal/entity"
	"github.com/sequring/sculptor/internal/usecase"
	v1 "k8s.io/api/core/v1"
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

	wantMainContainer := []OutputContainer{
		{
			Name: "main-app",
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{"cpu": resource.MustParse("100m"), "memory": resource.MustParse("256Mi")},
				Limits:   v1.ResourceList{"cpu": resource.MustParse("200m"), "memory": resource.MustParse("256Mi")},
			},
		},
	}
	wantInitContainer := []OutputContainer{
		{
			Name: "init-setup",
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{"cpu": resource.MustParse("100m"), "memory": resource.MustParse("64Mi")},
				Limits:   v1.ResourceList{"cpu": resource.MustParse("1000m"), "memory": resource.MustParse("64Mi")},
			},
		},
	}

	// Создаем презентер (writer и silent не важны для этого теста)
	p := NewYAMLPresenter(false, nil)

	// Тестируем основной контейнер
	gotMain, gotWarnings, err := p.buildOutputContainers([]usecase.NamedRecommendation{mainRec})
	if err != nil {
		t.Fatalf("BuildOutputContainers for main failed: %v", err)
	}
	if len(gotMain) != len(wantMainContainer) || gotMain[0].Name != wantMainContainer[0].Name ||
		!gotMain[0].Resources.Requests.Cpu().Equal(*wantMainContainer[0].Resources.Requests.Cpu()) ||
		!gotMain[0].Resources.Limits.Cpu().Equal(*wantMainContainer[0].Resources.Limits.Cpu()) ||
		!gotMain[0].Resources.Requests.Memory().Equal(*wantMainContainer[0].Resources.Requests.Memory()) ||
		!gotMain[0].Resources.Limits.Memory().Equal(*wantMainContainer[0].Resources.Limits.Memory()) {
		t.Errorf("main container mismatch:\ngot: %+v\nwant: %+v", gotMain, wantMainContainer)
	}
	if len(gotWarnings) != 2 { // OOM + Spikiness
		t.Errorf("expected 2 warnings for main container, got %d", len(gotWarnings))
	}

	// Тестируем init-контейнер
	gotInit, gotWarnings, err := p.buildOutputContainers([]usecase.NamedRecommendation{initRec})
	if err != nil {
		t.Fatalf("BuildOutputContainers for init failed: %v", err)
	}
	if len(gotInit) != len(wantInitContainer) || gotInit[0].Name != wantInitContainer[0].Name ||
		!gotInit[0].Resources.Requests.Cpu().Equal(*wantInitContainer[0].Resources.Requests.Cpu()) ||
		!gotInit[0].Resources.Limits.Cpu().Equal(*wantInitContainer[0].Resources.Limits.Cpu()) ||
		!gotInit[0].Resources.Requests.Memory().Equal(*wantInitContainer[0].Resources.Requests.Memory()) ||
		!gotInit[0].Resources.Limits.Memory().Equal(*wantInitContainer[0].Resources.Limits.Memory()) {
		t.Errorf("init container mismatch:\ngot: %+v\nwant: %+v", gotInit, wantInitContainer)
	}
	if len(gotWarnings) != 0 {
		t.Errorf("expected 0 warnings for init container, got %d", len(gotWarnings))
	}
}
