package k8s

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/sequring/sculptor/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type Client struct {
	Clientset  *kubernetes.Clientset
	RESTConfig *rest.Config
}

type Gateway struct {
	clientset *kubernetes.Clientset
}

func NewClient(cfg *config.Data) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: cfg.Context}

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return &Client{Clientset: clientset, RESTConfig: restConfig}, nil
}

func NewGateway(clientset *kubernetes.Clientset) *Gateway {
	return &Gateway{clientset: clientset}
}

func (g *Gateway) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return g.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (g *Gateway) CheckForOOMKilledEvents(ctx context.Context, d *appsv1.Deployment, targetContainerName string) (bool, string, *resource.Quantity, error) {
	selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return false, "", nil, fmt.Errorf("failed to build selector from deployment spec: %w", err)
	}

	podList, err := g.clientset.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return false, "", nil, fmt.Errorf("failed to list pods for deployment: %w", err)
	}

	for _, pod := range podList.Items {
		fieldSelector := fmt.Sprintf("involvedObject.kind=Pod,involvedObject.name=%s,reason=OOMKilled", pod.Name)
		eventList, err := g.clientset.CoreV1().Events(d.Namespace).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
		if err != nil {
			log.Printf("Warning: could not get events for pod %s: %v", pod.Name, err)
			continue
		}

		if len(eventList.Items) > 0 {
			var currentLimit *resource.Quantity
			if len(pod.Spec.Containers) > 0 {
				if targetContainerName == "" {
					targetContainerName = pod.Spec.Containers[0].Name
				}
				for _, c := range pod.Spec.Containers {
					if c.Name == targetContainerName {
						if c.Resources.Limits != nil {
							if limit, ok := c.Resources.Limits[v1.ResourceMemory]; ok {
								currentLimit = &limit
							}
						}
						break
					}
				}
			}
			return true, pod.Name, currentLimit, nil
		}
	}

	return false, "", nil, nil
}

func (c *Client) StartPortForward(namespace, serviceName string, port int, stopCh, readyCh chan struct{}) error {
	svc, err := c.Clientset.CoreV1().Services(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("could not find service %s in namespace %s: %w", serviceName, namespace, err)
	}

	selector := metav1.ListOptions{LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: svc.Spec.Selector})}
	podList, err := c.Clientset.CoreV1().Pods(namespace).List(context.TODO(), selector)
	if err != nil {
		return fmt.Errorf("could not list pods for service %s: %w", serviceName, err)
	}
	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods found for service %s", serviceName)
	}

	var targetPod v1.Pod
	found := false
	for _, p := range podList.Items {
		if p.Status.Phase == v1.PodRunning {
			targetPod = p
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no running pods found for service %s", serviceName)
	}

	log.Printf("Starting port-forward to pod '%s' for service '%s'...\n", targetPod.Name, serviceName)

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, targetPod.Name)
	hostIP := strings.TrimLeft(c.RESTConfig.Host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(c.RESTConfig)
	if err != nil {
		return fmt.Errorf("failed to create round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: hostIP})
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	ports := []string{fmt.Sprintf("%d:%d", port, port)}

	fw, err := portforward.New(dialer, ports, stopCh, readyCh, out, errOut)
	if err != nil {
		return fmt.Errorf("failed to create port forwarder: %w", err)
	}

	return fw.ForwardPorts()
}
