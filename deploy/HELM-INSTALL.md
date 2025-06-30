# 🚀 Helm Installation Guide

This guide shows you how to deploy kubeflare-light to the `kubeflare` namespace using Helm.

## 📋 Prerequisites

- **Kubernetes cluster** (v1.19+)
- **Helm 3.0+** installed
- **kubectl** configured to access your cluster
- **Cloudflare API token** with Zone:Edit permissions

## ⚡ Quick Installation

### 1. Create the kubeflare namespace

```bash
kubectl create namespace kubeflare
```

### 2. Install with Helm (Direct Method)

```bash
# Basic installation with inline values
helm install kubeflare-light ./chart/kubeflare-light \
  --namespace kubeflare \
  --set cloudflare.apiToken="your-cloudflare-api-token-here" \
  --set zones.defaultZoneId="your-zone-id-here"
```

### 3. Install with Helm (Values File Method - Recommended)

```bash
# 1. Copy and edit the values file
cp deploy/helm-kubeflare-values.yaml my-values.yaml

# 2. Edit my-values.yaml with your configuration
# - Set cloudflare.apiToken
# - Set zones.defaultZoneId
# - Add domain mappings if needed

# 3. Install with values file
helm install kubeflare-light ./chart/kubeflare-light \
  --namespace kubeflare \
  --values my-values.yaml
```

## 🔧 Advanced Installation Options

### Using Existing Secret

If you already have a Cloudflare API token in a Kubernetes secret:

```bash
# Create secret first
kubectl create secret generic cloudflare-token \
  --from-literal=token="your-cloudflare-api-token" \
  --namespace kubeflare

# Install using existing secret
helm install kubeflare-light ./chart/kubeflare-light \
  --namespace kubeflare \
  --set cloudflare.existingSecret="cloudflare-token" \
  --set cloudflare.secretKey="token" \
  --set zones.defaultZoneId="your-zone-id-here"
```

### Local Development (Kind)

For local development with Kind cluster:

```bash
# Build and load image to Kind
make docker-build
kind load docker-image kubeflare-light:v1

# Install with local image
helm install kubeflare-light ./chart/kubeflare-light \
  --namespace kubeflare \
  --set image.pullPolicy=Never \
  --set cloudflare.apiToken="your-token" \
  --set zones.defaultZoneId="your-zone-id"
```

### Production Installation

For production deployment with custom resource limits:

```bash
helm install kubeflare-light ./chart/kubeflare-light \
  --namespace kubeflare \
  --values my-values.yaml \
  --set replicaCount=1 \
  --set resources.limits.cpu=200m \
  --set resources.limits.memory=256Mi \
  --set controller.maxConcurrentReconciles=3
```

## 📖 Configuration Examples

### Basic Configuration

```yaml
# my-values.yaml
cloudflare:
  apiToken: "your-cloudflare-api-token-here"

zones:
  defaultZoneId: "4b5c7c7ca06cddfe0fe680cf0b570920"
```

### Multi-Zone Configuration

```yaml
# my-values.yaml
cloudflare:
  apiToken: "your-cloudflare-api-token-here"

zones:
  defaultZoneId: "4b5c7c7ca06cddfe0fe680cf0b570920"
  mappings:
    example.com: "4b5c7c7ca06cddfe0fe680cf0b570920"
    api.example.com: "5c6d8d8db07dedfe0fe680cf0b570930"
    cdn.example.com: "6d7e9e9ec18fefefe0fe680cf0b570940"
```

### High Availability Configuration

```yaml
# my-values.yaml
cloudflare:
  apiToken: "your-cloudflare-api-token-here"

zones:
  defaultZoneId: "4b5c7c7ca06cddfe0fe680cf0b570920"

controller:
  maxConcurrentReconciles: 5
  leaderElection: true

resources:
  limits:
    cpu: 200m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

replicaCount: 1  # Leader election handles HA, keep at 1
```

## ✅ Verification

### Check Installation Status

```bash
# Check Helm release
helm status kubeflare-light -n kubeflare

# Check pods
kubectl get pods -n kubeflare

# Check logs
kubectl logs -f deployment/kubeflare-light-controller -n kubeflare
```

### Verify CRDs

```bash
# Check if CRDs are installed
kubectl get crd | grep cloudflare

# Should show:
# ratelimits.cloudflare.kubeflare.io
# wafrules.cloudflare.kubeflare.io
```

### Test Configuration

```bash
# Check ConfigMap
kubectl get configmap kubeflare-zones -n kubeflare -o yaml

# Check Secret
kubectl get secret -n kubeflare

# Check RBAC
kubectl get clusterrole | grep kubeflare
kubectl get clusterrolebinding | grep kubeflare
```

## 🎯 Post-Installation

### Create Your First Rate Limit

```bash
# Create example rate limit
cat <<EOF | kubectl apply -f -
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: api-rate-limit
  namespace: kubeflare
spec:
  zoneId: "example.com"  # Will be resolved via ConfigMap
  description: "API rate limiting"
  match:
    request:
      urlPattern: "/api/v1/*"
      methods: ["GET", "POST"]
  threshold: 100
  period: 60
  action:
    mode: "block"
    timeout: 300
EOF
```

### Check Rate Limit Status

```bash
# List rate limits
kubectl get ratelimits -n kubeflare

# Describe rate limit
kubectl describe ratelimit api-rate-limit -n kubeflare
```

## 🔧 Troubleshooting

### Common Issues

#### 1. API Token Issues
```bash
# Check if secret exists and has correct key
kubectl get secret cloudflare-api-token -n kubeflare -o yaml

# Test API token manually
curl -X GET "https://api.cloudflare.com/client/v4/zones" \
  -H "Authorization: Bearer your-token-here"
```

#### 2. Zone ID Issues
```bash
# Check ConfigMap configuration
kubectl get configmap kubeflare-zones -n kubeflare -o yaml

# Check controller logs for zone resolution errors
kubectl logs deployment/kubeflare-light-controller -n kubeflare | grep -i zone
```

#### 3. RBAC Issues
```bash
# Check if service account has correct permissions
kubectl auth can-i create ratelimits --as=system:serviceaccount:kubeflare:kubeflare-light-controller

# Check ClusterRoleBinding
kubectl describe clusterrolebinding kubeflare-light-manager-rolebinding
```

#### 4. Controller Not Starting
```bash
# Check pod status
kubectl get pods -n kubeflare

# Check pod events
kubectl describe pod -l app.kubernetes.io/component=controller -n kubeflare

# Check controller logs
kubectl logs deployment/kubeflare-light-controller -n kubeflare
```

### Debug Commands

```bash
# Get all kubeflare resources
kubectl get all -n kubeflare

# Check controller health
kubectl get deployment kubeflare-light-controller -n kubeflare

# Port forward to access metrics
kubectl port-forward deployment/kubeflare-light-controller 8080:8080 -n kubeflare
# Then visit http://localhost:8080/metrics

# Port forward to access health checks
kubectl port-forward deployment/kubeflare-light-controller 8081:8081 -n kubeflare
# Then visit http://localhost:8081/healthz
```

## 🗑️ Uninstallation

### Remove Helm Release

```bash
# Delete all custom resources first (triggers finalizers)
kubectl delete ratelimits --all -n kubeflare
kubectl delete wafrules --all -n kubeflare

# Uninstall Helm release
helm uninstall kubeflare-light -n kubeflare

# Delete namespace
kubectl delete namespace kubeflare
```

### Remove CRDs (Optional)

```bash
# Remove CRDs cluster-wide
kubectl delete crd ratelimits.cloudflare.kubeflare.io
kubectl delete crd wafrules.cloudflare.kubeflare.io
```

## 📚 Additional Resources

- [Main README](../README.md)
- [API Reference](../docs/api-reference.md)
- [Examples](../examples/)
- [Troubleshooting Guide](../docs/troubleshooting.md)
- [Helm Chart Documentation](../chart/kubeflare-light/README.md)