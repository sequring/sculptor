
# Sculptor

Sculptor is a command-line tool for Kubernetes SREs and developers that analyzes resource usage of Deployments and generates optimal CPU and Memory configurations. Instead of just suggesting annotations, it produces a clean, ready-to-use YAML snippet for your GitOps workflow.


## Features

-   **Intelligent Recommendations:** Calculates optimal resource requests and limits based on historical Prometheus data.
-   **Conservative Memory Sizing:** Uses the p99 of memory usage plus a buffer to prevent OOMKills.
-   **Flexible CPU Sizing:** Uses p90 for requests (guaranteed CPU) and p99 for limits (burstable CPU).
-   **GitOps-Ready Output:** Generates a clean YAML snippet of the `resources` block, ready to be pasted into your Deployment manifest.
-   **Flexible Analysis:** Analyze resource usage over configurable time ranges (e.g., last 7 days, 24 hours, or 1 hour).
-   **Init Container Support:** Analyze and generate recommendations for both main and init containers.
-   **Self-Contained:** Automatically port-forwards to your Prometheus instance, requiring zero setup from the user.
-   **Config-Driven:** Uses a simple `config.toml` file for environment-specific settings.

## Installation

### From Releases

You can download the latest pre-compiled binary for your operating system from the [GitHub Releases](https://github.com/sequring/sculptor/releases) page.

Once downloaded, extract the archive and move the `sculptor` binary to a directory in your `PATH`, for example:

```bash
# For Linux/macOS
sudo mv ./sculptor /usr/local/bin/sculptor
```

### From Source

If you have Go installed, you can build it from the source:
```bash
git clone https://github.com/sequring/sculptor.git
cd sculptor
go build -o sculptor ./cmd/sculptor-cli
sudo mv ./sculptor /usr/local/bin/
```

## Configuration

Sculptor uses a `config.toml` file to manage settings. Create this file in the same directory where you run the command, or specify a path using the `--config` flag.

Here is a sample `config.toml`:

```toml
# config.toml

# (Optional) The name of the kubeconfig context to use.
# If empty, the currently active context will be used.
context = "my-prod-cluster" 

# (Optional) The absolute path to the kubeconfig file.
# If empty, the default path (~/.kube/config) will be used.
kubeconfig = "" 

# The default time range for Prometheus queries.
# Can be overridden by the --range flag.
# Valid units: s (seconds), m (minutes), h (hours), d (days), w (weeks), y (years).
range = "7d"

# Prometheus connection settings for automatic port-forwarding.
[prometheus]
  # Namespace where the Prometheus service is located.
  namespace = "monitoring"
  
  # Name of the Prometheus service.
  service = "kube-prometheus-stack-prometheus"
  
  # The local port to forward to.
  port = 9090
```

## Usage

The primary command requires you to specify the namespace and name of the Deployment you wish to analyze.

```bash
sculptor --namespace <namespace> --deployment <deployment-name> [flags]
```

### Examples

**1. Analyze a deployment using defaults from `config.toml`:**

```bash
sculptor --namespace=prod --deployment=backend-api
```

**2. Analyze a deployment over the last 24 hours, overriding the config file:**

```bash
sculptor --namespace=prod --deployment=backend-api --range=24h
```

**3. Target a specific container within a multi-container Pod:**

If your deployment has multiple containers (e.g., an application and a sidecar), you can specify which one to apply resources to.

```bash
sculptor --namespace=prod --deployment=web-server --container=php-fpm
```

**4. Analyze init containers:**

```bash
# Analyze only init containers
sculptor --namespace=prod --deployment=web-server --target=init

# Analyze both main and init containers (default)
sculptor --namespace=prod --deployment=web-server --target=all
```

**5. Use a different Kubernetes context:**

```bash
sculptor --namespace=staging --deployment=user-service --context=my-staging-cluster
```

### Example Output

The tool will print a clean YAML snippet that you can directly paste into the `spec.template.spec` section of your Deployment manifest.

```bash
$ sculptor --namespace=dev --deployment=api


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
... [log messages] ...

--- Recommended Resource Snippet (paste into your Deployment YAML) ---
containers:
- name: api
  resources:
    limits:
      cpu: 925m
      memory: 512Mi
    requests:
      cpu: 800m
      memory: 512Mi
```

### All Flags

| Flag         | Description                                                                              | Default                          |
|--------------|------------------------------------------------------------------------------------------|----------------------------------|
| `--namespace`  | The namespace of the deployment.                                                         | `default`                        |
| `--deployment` | The name of the deployment to analyze. **(Required)**                                    |                                  |
| `--container`  | The name of the container to apply resources to.                                         | The first container in the Pod.  |
| `--target`     | Which containers to analyze: `main`, `init`, or `all`.                                   | `all`                            |
| `--range`      | The time range for Prometheus analysis (e.g., `7d`, `24h`). Overrides the config file.     | `7d`                             |
| `--context`    | The name of the kubeconfig context to use. Overrides the config file.                    | Active context                   |
| `--kubeconfig` | The absolute path to the kubeconfig file. Overrides the config file.                     | `~/.kube/config`                 |
| `--config`     | The path to the `config.toml` file.                                                      | `config.toml`                    |
| `--silent`     | Disable all logs and logo output, only show the YAML output.                             | `false`                          |

---

### How it works
Sculptor performs the following calculations based on historical data from Prometheus:
- **Memory Request & Limit:** `p99(memory_usage) + 20% buffer`. This ensures a `Guaranteed` QoS class for memory, preventing OOMKills.
- **CPU Request:** `p90(cpu_usage)`. This provides a stable, guaranteed amount of CPU for normal operations.
- **CPU Limit:** `p99(cpu_usage)`. This allows the application to burst and handle peak loads without throttling.