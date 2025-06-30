# Kubeflare Light

A Kubernetes operator for managing Cloudflare rate limiting rules and WAF rules using Custom Resource Definitions (CRDs).

## Overview

Kubeflare Light allows you to manage your Cloudflare security rules directly from Kubernetes using familiar kubectl commands and YAML manifests. The operator watches for changes to RateLimit and WAFRule custom resources and automatically synchronizes them with your Cloudflare zones.

## Features

- **Rate Limit Management**: Create, update, and delete Cloudflare rate limiting rules
- **WAF Rule Management**: Manage Cloudflare WAF rules using the latest Rulesets API
- **Kubernetes Native**: Full integration with Kubernetes RBAC, events, and status reporting
- **Declarative Configuration**: Define your security rules as YAML manifests
- **Status Reporting**: Real-time status updates and condition reporting
- **Finalizer Support**: Proper cleanup of Cloudflare resources when Kubernetes resources are deleted

## Prerequisites

- Kubernetes cluster (v1.19+)
- Cloudflare API token with Zone:Edit permissions
- Go 1.24+ (for development)
- Docker for building container images
- Access to a container registry (Docker Hub, Google Container Registry, etc.)

## Installation

### 1. Clone the Repository

```bash
git clone https://github.com/wzygadlo/kubeflare-light.git
cd kubeflare-light
```

### 2. Configure Your Environment

Edit the Makefile to set your container registry and other variables:

```bash
# Set your registry (replace docker.io/your-username)
REGISTRY=docker.io/your-username
IMAGE_NAME=kubeflare-light
TAG=latest
```

### 3. Generate Your Cloudflare API Token

1. Log in to the Cloudflare dashboard
2. Navigate to "My Profile" > "API Tokens"
3. Create a new token with Zone:Edit permissions for the zones you want to manage

### 4. Build and Deploy

You can build and deploy the operator using the provided Makefile:

```bash
# Build the binary, create Docker image, push to registry, and deploy to Kubernetes
make all CLOUDFLARE_API_TOKEN=your-token

# Or perform the steps individually:
make build            # Build the Go binary
make docker-build     # Build the Docker image
make docker-push      # Push the image to your registry
make deploy CLOUDFLARE_API_TOKEN=your-token  # Deploy to Kubernetes
```

### 5. Verify the Installation

Check that the operator is running:

```bash
kubectl get pods -n kubeflare-light-system
```

## Usage

### Creating a Rate Limit Rule

Create a YAML file (e.g., `rate-limit.yaml`) for your rate limit rule:

```yaml
apiVersion: cloudflare.io/v1alpha1
kind: RateLimit
metadata:
  name: api-rate-limit
  namespace: default
spec:
  zoneId: "your-zone-id-here"
  description: "Rate limit for API endpoints to prevent abuse"
  match:
    request:
      methods: ["GET", "POST", "PUT", "DELETE"]
      schemes: ["https"]
      urlPattern: "/api/*"
    response:
      statuses: [200, 201, 202]
  threshold: 100
  period: 60
  action:
    mode: "challenge"
    timeout: 86400
  disabled: false
```

Apply the file to your cluster:

```bash
kubectl apply -f rate-limit.yaml
```

### Creating a WAF Rule

Create a YAML file (e.g., `waf-rule.yaml`) for your WAF rule:

```yaml
apiVersion: cloudflare.io/v1alpha1
kind: WAFRule
metadata:
  name: block-malicious-ips
  namespace: default
spec:
  zoneId: "your-zone-id-here"
  description: "Block known malicious IP addresses and suspicious traffic"
  expression: '(ip.src in {192.0.2.1 192.0.2.2 198.51.100.0/24}) or (cf.threat_score gt 14)'
  action: "block"
  priority: 1
  paused: false
  actionParameters:
    response:
      statusCode: 403
      contentType: "application/json"
      content: '{"error": "Access denied", "reason": "Your IP has been blocked due to suspicious activity"}'
```

Apply the file to your cluster:

```bash
kubectl apply -f waf-rule.yaml
```

### Deploying the Example Resources

You can deploy the example resources provided in the `examples` directory:

```bash
make generate-examples
# or
kubectl apply -f examples/
```

### Checking Rule Status

Check the status of your rate limit rule:

```bash
kubectl get ratelimit
```

Check the status of your WAF rule:

```bash
kubectl get wafrule
```

### Cleaning Up

To remove the operator from your cluster:

```bash
make undeploy
```

## Development

### Building Locally

```bash
make build
```

### Running Tests

```bash
make test
```

### Generating Manifests

To regenerate CRD manifests:

```bash
make manifests
```

## Troubleshooting

### Common Issues

1. **Image Pull Errors**: Make sure your image is accessible to your Kubernetes cluster
2. **API Token Errors**: Verify your Cloudflare API token has the required permissions
3. **Zone ID Errors**: Confirm the Zone ID is correct for your domain

### Checking Operator Logs

```bash
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller -f
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.
