package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestNewGateway(t *testing.T) {
	// Arrange
	mockCs := fake.NewSimpleClientset() // Use fake clientset
	logger := slog.Default()

	// Act
	gateway := NewGateway(mockCs, logger)

	// Assert
	if gateway == nil {
		t.Fatal("Expected gateway not to be nil")
	}
	if gateway.clientset != mockCs {
		t.Errorf("Expected clientset to be %p, got %p", mockCs, gateway.clientset)
	}
	if gateway.logger != logger {
		t.Errorf("Expected logger to be %p, got %p", logger, gateway.logger)
	}
}

func TestGateway_GetDeployment(t *testing.T) {
	// Arrange
	expectedDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "test-namespace"},
	}

	mockCs := fake.NewSimpleClientset()
	mockCs.PrependReactor("get", "deployments", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		getAction := action.(k8stesting.GetAction)
		if getAction.GetName() == "test-deployment" {
			return true, expectedDeployment, nil
		}
		return true, nil, fmt.Errorf("deployment not found")
	})

	logger := slog.Default()
	gateway := NewGateway(mockCs, logger)

	// Act
	deployment, err := gateway.GetDeployment(context.Background(), "test-namespace", "test-deployment")

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if deployment == nil {
		t.Fatal("Expected deployment, got nil")
	}
	if deployment.Name != expectedDeployment.Name {
		t.Errorf("Expected deployment name %s, got %s", expectedDeployment.Name, deployment.Name)
	}
	if deployment.Namespace != expectedDeployment.Namespace {
		t.Errorf("Expected deployment namespace %s, got %s", expectedDeployment.Namespace, deployment.Namespace)
	}
}