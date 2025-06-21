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
	Target     string
	Silent     bool
	Verbose    bool
	Prometheus struct {
		URL       string `mapstructure:"url"`
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
	pflag.String("container", "", "The name of the container to apply resources to (defaults to all containers)")
	pflag.String("target", "all", "The target for analysis: 'all' for all containers, 'main' for primary containers, or 'init' for init containers")
	pflag.Bool("version", false, "Print version information and exit")
	pflag.Bool("silent", false, "Disable all logs and logo output, only show the YAML output")
	pflag.Bool("verbose", false, "Enable debug logging")

	viper.BindPFlag("kubeconfig", pflag.Lookup("kubeconfig"))
	viper.BindPFlag("context", pflag.Lookup("context"))
	viper.BindPFlag("range", pflag.Lookup("range"))
	viper.BindPFlag("namespace", pflag.Lookup("namespace"))
	viper.BindPFlag("deployment", pflag.Lookup("deployment"))
	viper.BindPFlag("container", pflag.Lookup("container"))
	viper.BindPFlag("target", pflag.Lookup("target"))
	viper.BindPFlag("silent", pflag.Lookup("silent"))
	viper.BindPFlag("verbose", pflag.Lookup("verbose"))

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
		return &cfg, nil
	}

	if cfg.Deployment == "" {
		return nil, fmt.Errorf("--deployment flag is required")
	}

	validRangeRegex := regexp.MustCompile(`^[1-9][0-9]*[smhdwy]$`)
	if !validRangeRegex.MatchString(cfg.Range) {
		return nil, fmt.Errorf("invalid format for 'range': %s. Use Prometheus range format like '1h', '7d', '2w'", cfg.Range)
	}

	if cfg.Target != "all" && cfg.Target != "main" && cfg.Target != "init" {
		return nil, fmt.Errorf("invalid value for --target: must be 'all', 'main', or 'init'")
	}

	return &cfg, nil
}
