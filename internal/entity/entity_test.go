package entity

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCPURecommendation(t *testing.T) {
	req := resource.MustParse("100m")
	limit := resource.MustParse("200m")

	cpuRec := CPURecommendation{
		Request:          &req,
		Limit:            &limit,
		SpikinessWarning: true,
	}

	if cpuRec.Request.String() != "100m" {
		t.Errorf("Expected CPU Request to be 100m, got %s", cpuRec.Request.String())
	}
	if cpuRec.Limit.String() != "200m" {
		t.Errorf("Expected CPU Limit to be 200m, got %s", cpuRec.Limit.String())
	}
	if !cpuRec.SpikinessWarning {
		t.Errorf("Expected SpikinessWarning to be true, got %t", cpuRec.SpikinessWarning)
	}
}

func TestRecommendation(t *testing.T) {
	mem := resource.MustParse("512Mi")
	req := resource.MustParse("100m")
	limit := resource.MustParse("200m")

	cpuRec := CPURecommendation{
		Request:          &req,
		Limit:            &limit,
		SpikinessWarning: false,
	}

	recommendation := Recommendation{
		Memory:      &mem,
		CPU:         &cpuRec,
		IsOOMKilled: true,
	}

	if recommendation.Memory.String() != "512Mi" {
		t.Errorf("Expected Memory to be 512Mi, got %s", recommendation.Memory.String())
	}
	if recommendation.CPU.Request.String() != "100m" {
		t.Errorf("Expected CPU Request to be 100m, got %s", recommendation.CPU.Request.String())
	}
	if recommendation.CPU.Limit.String() != "200m" {
		t.Errorf("Expected CPU Limit to be 200m, got %s", recommendation.CPU.Limit.String())
	}
	if recommendation.CPU.SpikinessWarning {
		t.Errorf("Expected SpikinessWarning to be false, got %t", recommendation.CPU.SpikinessWarning)
	}
	if !recommendation.IsOOMKilled {
		t.Errorf("Expected IsOOMKilled to be true, got %t", recommendation.IsOOMKilled)
	}
}
