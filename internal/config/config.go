package config

import (
	"fmt"
	"regexp"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Data struct {
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

func Load() (*Data, error) {
	pflag.String("kubeconfig", "", "path to kubeconfig file")
	pflag.String("context", "", "the name of the kubeconfig context to use")
	pflag.String("config", "config.toml", "path to config file")
	pflag.String("range", "7d", "analysis range for prometheus (e.g. 7d, 24h, 1h)")
	pflag.String("namespace", "default", "The namespace of the deployment")
	pflag.String("deployment", "", "The name of the deployment to analyze")
	pflag.String("container", "", "The name of the container to apply resources to (defaults to the first container)")
	pflag.Bool("version", false, "Print version information and exit")

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
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var cfg Data
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config into struct: %w", err)
	}

	showVersion, _ := pflag.CommandLine.GetBool("version")
	if showVersion {
		// This is a special case handled in main.go, so we can return early.
		// A more advanced CLI might handle this directly here.
		return nil, nil // Returning nil, nil signals main to print version and exit.
	}

	if cfg.Deployment == "" {
		return nil, fmt.Errorf("--deployment flag is required")
	}

	validRangeRegex := regexp.MustCompile(`^[1-9][0-9]*[smhdwy]$`)
	if !validRangeRegex.MatchString(cfg.Range) {
		return nil, fmt.Errorf("invalid format for 'range': %s. Use Prometheus range format like '1h', '7d', '2w'", cfg.Range)
	}

	return &cfg, nil
}
