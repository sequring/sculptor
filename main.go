package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/BurntSushi/toml"
	promapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/yaml"
)


type Config struct {
	Kubeconfig string
	Context    string
	Range      string
	Namespace  string
	Deployment string
	Container  string
	Prometheus struct {
		Namespace string
		Service   string
		Port      int
	}
}

type OutputYAML struct {
	Containers []OutputContainer `yaml:"containers"`
}

type OutputContainer struct {
	Name      string                  `yaml:"name"`
	Resources v1.ResourceRequirements `yaml:"resources"`
}

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

var cfg Config

func main() {
	printLogo()
	initConfig()


	if cfg.Deployment == "" {
		log.Fatal("Error: --deployment flag is required.")
	}

	validRangeRegex := regexp.MustCompile(`^[1-9][0-9]*[smhdwy]$`)
	if !validRangeRegex.MatchString(cfg.Range) {
		log.Fatalf("Invalid format for 'range': %s. Use Prometheus range format like '1h', '7d', '2w'.", cfg.Range)
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: cfg.Context}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes clientset: %s", err.Error())
	}

	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	defer close(stopCh)

	go func() {
		err := startPortForward(config, clientset, cfg.Prometheus.Namespace, cfg.Prometheus.Service, cfg.Prometheus.Port, stopCh, readyCh)
		if err != nil {
			log.Fatalf("Port-forwarding failed: %v", err)
		}
	}()

	select {
	case <-readyCh:
		log.Println("Port-forwarding is ready.")
	case <-time.After(30 * time.Second):
		log.Fatal("Port-forwarding timed out.")
	}

	prometheusURL := fmt.Sprintf("http://localhost:%d", cfg.Prometheus.Port)
	promClient, err := promapi.NewClient(promapi.Config{Address: prometheusURL})
	if err != nil {
		log.Fatalf("Error creating Prometheus client: %s", err.Error())
	}
	promAPI := prometheusv1.NewAPI(promClient)

	log.Printf("Analyzing deployment '%s' in namespace '%s' over the last %s...\n", cfg.Deployment, cfg.Namespace, cfg.Range)

	originalDeployment, err := clientset.AppsV1().Deployments(cfg.Namespace).Get(context.TODO(), cfg.Deployment, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Failed to get deployment '%s' in namespace '%s': %v", cfg.Deployment, cfg.Namespace, err)
	}
	//log.Println("Deployment found.")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	memQuery := fmt.Sprintf(`max(quantile_over_time(0.99, sum(container_memory_working_set_bytes{namespace="%s", pod=~"^%s-.*", container!=""}) by (pod, namespace)[%s:]))`, cfg.Namespace, cfg.Deployment, cfg.Range)
	memP99, err := executePrometheusQuery(ctx, promAPI, memQuery)
	if err != nil {
		log.Fatalf("Memory query failed: %v", err)
	}

	cpuRequestQuery := fmt.Sprintf(`max(quantile_over_time(0.90, sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container!=""}[5m])) by (pod, namespace)[%s:1m]))`, cfg.Namespace, cfg.Deployment, cfg.Range)
	cpuP90, err := executePrometheusQuery(ctx, promAPI, cpuRequestQuery)
	if err != nil {
		log.Fatalf("CPU request query failed: %v", err)
	}

	cpuLimitQuery := fmt.Sprintf(`max(quantile_over_time(0.99, sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"^%s-.*", container!=""}[5m])) by (pod, namespace)[%s:1m]))`, cfg.Namespace, cfg.Deployment, cfg.Range)
	cpuP99, err := executePrometheusQuery(ctx, promAPI, cpuLimitQuery)
	if err != nil {
		log.Fatalf("CPU limit query failed: %v", err)
	}

	if memP99 == 0 && cpuP90 == 0 && cpuP99 == 0 {
		log.Printf("No metric data found in Prometheus for this deployment over the last %s.", cfg.Range)
		os.Exit(0)
	}

	memRecommendationBytes := memP99 * 1.2
	memRecommendation := resource.NewQuantity(int64(memRecommendationBytes), resource.BinarySI)
	cpuRequestRecommendation := resource.NewMilliQuantity(int64(cpuP90*1000), resource.DecimalSI)
	cpuLimitRecommendation := resource.NewMilliQuantity(int64(cpuP99*1000), resource.DecimalSI)

	log.Printf("Calculated recommendations: Memory=%s, CPU Request=%s, CPU Limit=%s", formatMemoryHumanReadable(memRecommendation), cpuRequestRecommendation.String(), cpuLimitRecommendation.String())

	var targetContainerName string
	if cfg.Container != "" {
		found := false
		for _, c := range originalDeployment.Spec.Template.Spec.Containers {
			if c.Name == cfg.Container {
				targetContainerName = c.Name
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("Error: container '%s' not found in deployment", cfg.Container)
		}
	} else {
		if len(originalDeployment.Spec.Template.Spec.Containers) > 0 {
			targetContainerName = originalDeployment.Spec.Template.Spec.Containers[0].Name
			log.Printf("No --container specified, targeting the first container: '%s'\n", targetContainerName)
		} else {
			log.Fatal("Error: no containers found in deployment spec")
		}
	}

	memString := formatMemoryHumanReadable(memRecommendation)
	prettyMem, err := resource.ParseQuantity(memString)
	if err != nil {
		log.Fatalf("Internal error parsing pretty memory: %v", err)
	}
	prettyCpuReq, err := resource.ParseQuantity(cpuRequestRecommendation.String())
	if err != nil {
		log.Fatalf("Internal error parsing pretty CPU request: %v", err)
	}
	prettyCpuLim, err := resource.ParseQuantity(cpuLimitRecommendation.String())
	if err != nil {
		log.Fatalf("Internal error parsing pretty CPU limit: %v", err)
	}

	outputData := OutputYAML{
		Containers: []OutputContainer{
			{
				Name: targetContainerName,
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    prettyCpuReq,
						v1.ResourceMemory: prettyMem,
					},
					Limits: v1.ResourceList{
						v1.ResourceCPU:    prettyCpuLim,
						v1.ResourceMemory: prettyMem,
					},
				},
			},
		},
	}

	yamlBytes, err := yaml.Marshal(outputData)
	if err != nil {
		log.Fatalf("Failed to marshal snippet to YAML: %v", err)
	}

	fmt.Println("\n--- Recommended Resource Snippet (paste into your Deployment YAML) ---")
	fmt.Print(string(yamlBytes))
}

func initConfig() {
	pflag.String("kubeconfig", "", "path to kubeconfig file")
	pflag.String("context", "", "the name of the kubeconfig context to use")
	pflag.String("config", "config.toml", "path to config file")
	pflag.String("range", "7d", "analysis range for prometheus (e.g. 7d, 24h, 1h)")
	pflag.String("namespace", "default", "The namespace of the deployment")
	pflag.String("deployment", "", "The name of the deployment to analyze")
	pflag.String("container", "", "The name of the container to apply resources to (defaults to the first container)")
	viper.BindPFlag("kubeconfig", pflag.Lookup("kubeconfig"))
	viper.BindPFlag("context", pflag.Lookup("context"))
	viper.BindPFlag("range", pflag.Lookup("range"))
	viper.BindPFlag("namespace", pflag.Lookup("namespace"))
	viper.BindPFlag("deployment", pflag.Lookup("deployment"))
	viper.BindPFlag("container", pflag.Lookup("container"))
	pflag.Parse()
	configPath, _ := pflag.CommandLine.GetString("config")
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || configPath != "config.toml" {
			log.Fatalf("Error reading config file: %s\n", err)
		}
	}
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Unable to decode config into struct, %v", err)
	}
}

func startPortForward(config *rest.Config, clientset *kubernetes.Clientset, namespace, serviceName string, port int, stopCh, readyCh chan struct{}) error {
	svc, err := clientset.CoreV1().Services(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("could not find service %s in namespace %s: %w", serviceName, namespace, err)
	}
	selector := metav1.ListOptions{LabelSelector: metav1.FormatLabelSelector(
		&metav1.LabelSelector{MatchLabels: svc.Spec.Selector},
	)}
	podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), selector)
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
	hostIP := strings.TrimLeft(config.Host, "https://")
	transport, upgrader, err := spdy.RoundTripperFor(config)
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

func executePrometheusQuery(ctx context.Context, api prometheusv1.API, query string) (float64, error) {
	log.Printf("Executing query to Prometheus\n")
	result, warnings, err := api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	if len(warnings) > 0 {
		log.Printf("Prometheus query returned warnings: %v\n", warnings)
	}
	vector, ok := result.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %s", result.Type().String())
	}
	if vector.Len() == 0 {
		log.Println("Query returned no data.")
		return 0, nil
	}
	value := float64(vector[0].Value)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, nil
	}
	return value, nil
}

func formatMemoryHumanReadable(q *resource.Quantity) string {
	const (
		Ki = 1024
		Mi = 1024 * Ki
		//Gi = 1024 * Mi
	)
	bytes := q.Value()
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

func printLogo() {
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
	fmt.Println(logo)
}
