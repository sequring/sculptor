package entity

import "k8s.io/apimachinery/pkg/api/resource"

type Recommendation struct {
	Memory      *resource.Quantity
	CPU         *CPURecommendation
	IsOOMKilled bool
}

type CPURecommendation struct {
	Request          *resource.Quantity
	Limit            *resource.Quantity
	SpikinessWarning bool
}
