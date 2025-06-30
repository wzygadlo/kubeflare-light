# 🛡️ Kubeflare Light

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.19+-blue.svg)](https://kubernetes.io/)
[![Cloudflare SDK](https://img.shields.io/badge/Cloudflare%20SDK-v4.5.1-orange.svg)](https://github.com/cloudflare/cloudflare-go)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A production-ready Kubernetes controller for managing Cloudflare security resources using Custom Resource Definitions (CRDs). Built with the latest Cloudflare Rulesets API v4.5.1.

## 🚀 Overview

Kubeflare Light provides a Kubernetes-native way to manage your Cloudflare security configuration. Define rate limits and WAF rules as YAML manifests and let the controller handle the synchronization with Cloudflare's API.

### ✨ Key Benefits
- **GitOps Ready**: Version control your security policies
- **Kubernetes Native**: Full integration with K8s RBAC, events, and monitoring
- **Modern API**: Uses Cloudflare's latest Rulesets API (v4.5.1)
- **Production Ready**: Includes Helm charts, proper error handling, and observability

## 🎯 Features

### Core Functionality
- 🚦 **Rate Limit Management**: Create, update, and delete Cloudflare rate limiting rules
- 🛡️ **WAF Rule Management**: Manage Cloudflare WAF rules using the latest Rulesets API
- 🔧 **Kubernetes Native**: Full integration with Kubernetes RBAC, events, and status reporting
- 📝 **Declarative Configuration**: Define your security rules as YAML manifests
- 📊 **Status Reporting**: Real-time status updates and condition reporting
- 🔄 **Finalizer Support**: Proper cleanup of Cloudflare resources when Kubernetes resources are deleted

### Advanced Features
- 🌐 **Zone Resolution**: Flexible zone ID resolution via ConfigMap
- 📦 **Helm Charts**: Easy installation and configuration
- 🔍 **Expression Validation**: Validates Cloudflare expression syntax
- ⚡ **Performance Optimized**: Caching and concurrent processing
- 🛠️ **Developer Friendly**: Comprehensive APIs and examples

## 📋 Prerequisites

### Runtime Requirements
- 🎛️ **Kubernetes cluster** (v1.19+)
- 🔑 **Cloudflare API token** with Zone:Edit permissions
- 🌐 **Access to Cloudflare zones** you want to manage

### Development Requirements (Optional)
- 🐹 **Go 1.24+** (for building from source)
- 🐳 **Docker** (for building container images)
- 📦 **Container registry access** (Docker Hub, GCR, etc.)
- ⚓ **Helm 3.0+** (recommended for installation)

## 🚀 Quick Start

### Option 1: Helm Installation (Recommended)

1. **Add your Cloudflare API token:**
```bash
helm install kubeflare-light ./chart/kubeflare-light \
  --set cloudflare.apiToken="your-cloudflare-api-token-here" \
  --set zones.defaultZoneId="your-zone-id-here"
```

2. **Or use an existing secret:**
```bash
kubectl create secret generic cloudflare-api-token \
  --from-literal=token="your-cloudflare-api-token-here"

helm install kubeflare-light ./chart/kubeflare-light \
  --set cloudflare.existingSecret="cloudflare-api-token" \
  --set zones.defaultZoneId="your-zone-id-here"
```

### Option 2: Manual Installation

1. **Clone the repository:**
```bash
git clone https://github.com/wzygadlo/kubeflare-light.git
cd kubeflare-light
```

2. **Configure your environment:**
Edit the Makefile to set your container registry:
```bash
REGISTRY=docker.io/your-username
IMAGE_NAME=kubeflare-light
TAG=latest
```

3. **Generate your Cloudflare API Token:**
   - Log in to the Cloudflare dashboard
   - Navigate to "My Profile" > "API Tokens" 
   - Create a new token with Zone:Edit permissions

4. **Deploy:**
```bash
make all CLOUDFLARE_API_TOKEN=your-token
```

### ✅ Verify Installation

```bash
kubectl get pods -n kubeflare-light-system
kubectl get crd | grep cloudflare
```

## 📖 Usage Guide

### 🚦 Creating Rate Limit Rules

**Basic rate limiting example:**
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: api-rate-limit
  namespace: default
spec:
  # Zone ID or domain name (resolved via ConfigMap)
  zoneId: "example.com"
  description: "Rate limit for API endpoints"
  match:
    request:
      urlPattern: "/api/v1/*"
      methods: ["GET", "POST"]
  threshold: 100        # requests per period
  period: 60           # seconds
  action:
    mode: "block"
    timeout: 300
```

**Apply the rule:**
```bash
kubectl apply -f rate-limit.yaml
```

### 🛡️ Creating WAF Rules

**Block malicious IPs:**
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: block-malicious-ips
  namespace: default
spec:
  zoneId: "example.com"
  name: "Block Malicious IPs"
  description: "Block known malicious IP addresses"
  expression: 'ip.src in {192.0.2.1 198.51.100.1}'
  action: "block"
  priority: 1
```

**Apply the rule:**
```bash
kubectl apply -f waf-rule.yaml
```

### 🌐 Zone Configuration

Configure domain-to-zone mappings via ConfigMap:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubeflare-zones
  namespace: kubeflare-light-system
data:
  default: "4b5c7c7ca06cddfe0fe680cf0b570920"
  example.com: "4b5c7c7ca06cddfe0fe680cf0b570920"
  api.example.com: "5c6d8d8db07dedfe0fe680cf0b570930"
```

### 📊 Monitoring and Status

**Check resource status:**
```bash
kubectl get ratelimits,wafrules
kubectl describe ratelimit api-rate-limit
```

**View controller logs:**
```bash
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller -f
```

### 🗑️ Cleanup

**Remove specific rules:**
```bash
kubectl delete ratelimit api-rate-limit
kubectl delete wafrule block-malicious-ips
```

**Uninstall controller:**
```bash
helm uninstall kubeflare-light
# or
make undeploy
```

## 🔧 Development

### Local Development Setup
```bash
git clone https://github.com/wzygadlo/kubeflare-light.git
cd kubeflare-light

# Install dependencies
go mod download

# Build locally
make build

# Run tests
make test

# Generate manifests
make manifests
```

### Testing with Kind
```bash
# Create Kind cluster
kind create cluster --name kubeflare-test

# Build and load image
make docker-build
kind load docker-image kubeflare-light:v1 --name kubeflare-test

# Deploy with local image
helm install kubeflare-light ./chart/kubeflare-light \
  --set image.pullPolicy=Never \
  --set cloudflare.apiToken="your-token"
```

## 🔍 Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| 🚫 **Image Pull Errors** | Ensure image is accessible to cluster |
| 🔑 **API Token Errors** | Verify token has Zone:Edit permissions |
| 🌐 **Zone ID Errors** | Check zone ID format and ownership |
| ⚠️ **Expression Errors** | Validate Cloudflare expression syntax |

### Debugging Commands

**Check controller status:**
```bash
kubectl get pods -n kubeflare-light-system
kubectl describe deployment kubeflare-light-controller -n kubeflare-light-system
```

**View detailed logs:**
```bash
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller -f
```

**Check resource events:**
```bash
kubectl get events --sort-by=.metadata.creationTimestamp
```

## 📚 Documentation

| Type | Resource | Description |
|------|----------|-------------|
| **📚 Complete Docs** | [Documentation Hub](./docs/) | Comprehensive documentation portal |
| **📖 API Reference** | [API Docs](./docs/api-reference.md) | Complete CRD specification |
| **🔧 Developer Guide** | [Dev Guide](./docs/developer-guide.md) | Architecture and development |
| **🛡️ Security Guide** | [Security](./docs/security.md) | Security best practices |
| **🎯 Examples** | [Examples](./examples/) | Ready-to-use configurations |
| **⚡ Quick Deploy** | [Helm Chart](./chart/kubeflare-light/) | Production deployment |

## 🤝 Contributing

Contributions are welcome! Please read our [Contributing Guidelines](./CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## ⭐ Acknowledgments

- [Cloudflare Go SDK](https://github.com/cloudflare/cloudflare-go) for the excellent API client
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) for the Kubernetes controller framework
- The Kubernetes community for the amazing ecosystem
