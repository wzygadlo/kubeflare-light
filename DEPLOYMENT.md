# Kubeflare Light Deployment Guide

This guide provides detailed instructions for building and deploying the Kubeflare Light operator to your Kubernetes cluster.

## Prerequisites

Before proceeding, ensure you have:

1. **Docker** installed on your local machine
2. **kubectl** configured to connect to your Kubernetes cluster
3. **Go 1.24+** installed (for development)
4. Access to a **container registry** (Docker Hub, Google Container Registry, etc.)
5. A **Cloudflare API token** with Zone:Edit permissions

## Step 1: Configure Your Environment

First, you need to update the Makefile to point to your container registry and specify other deployment parameters.

Edit the Makefile and update these variables at the top:

```makefile
REGISTRY ?= docker.io/your-username  # Change this to your registry
IMAGE_NAME ?= kubeflare-light
TAG ?= latest
NAMESPACE ?= kubeflare-light-system
CLOUDFLARE_API_TOKEN ?= your-cloudflare-api-token  # Replace with your token or set via environment
```

## Step 2: Build the Operator

Build the operator binary, Docker image, and push it to your registry:

```bash
# Build the Go binary
make build

# Build and tag the Docker image
make docker-build

# Push the Docker image to your registry
make docker-push
```

Alternatively, you can perform all these steps in one command:

```bash
make build docker-build docker-push
```

## Step 3: Deploy to Kubernetes

### 3.1: Create the Namespace

Create the Kubernetes namespace where the operator will be deployed:

```bash
make create-namespace
```

### 3.2: Install CRDs

Install the Custom Resource Definitions for RateLimit and WAFRule:

```bash
make install-crds
```

### 3.3: Apply RBAC Resources

Apply the necessary RBAC resources (ServiceAccount, Role, RoleBinding):

```bash
make apply-rbac
```

### 3.4: Create the Cloudflare API Token Secret

Create a Kubernetes secret containing your Cloudflare API token:

```bash
# Replace with your actual token
make create-secret CLOUDFLARE_API_TOKEN=your-cloudflare-api-token
```

### 3.5: Deploy the Operator

Deploy the operator to your Kubernetes cluster:

```bash
make apply-deployment
```

### 3.6: All-in-One Deployment

Alternatively, you can perform all the deployment steps in one command:

```bash
make deploy CLOUDFLARE_API_TOKEN=your-cloudflare-api-token
```

## Step 4: Verify the Deployment

Check that the operator is running successfully:

```bash
kubectl get pods -n kubeflare-light-system
```

You should see the operator pod running:

```
NAME                                         READY   STATUS    RESTARTS   AGE
kubeflare-light-controller-abc123def-xyz     1/1     Running   0          30s
```

Check the operator logs:

```bash
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller -f
```

## Step 5: Deploy Example Resources

Deploy the example resources to test the operator:

```bash
# Make sure to update the examples with your Cloudflare Zone ID first
make generate-examples
```

Verify that the resources were created:

```bash
kubectl get ratelimit
kubectl get wafrule
```

## Troubleshooting

### Image Pull Errors

If Kubernetes cannot pull your Docker image, check:

1. Is your image pushed to the registry?
2. Does your Kubernetes cluster have access to the registry?
3. Is the image name correctly specified in the deployment?

```bash
# Manually check if your image exists
docker pull ${REGISTRY}/${IMAGE_NAME}:${TAG}
```

### API Token Issues

If the operator cannot connect to Cloudflare:

1. Verify that your API token has the correct permissions
2. Check the operator logs for API-related errors
3. Verify the secret was created correctly:

```bash
kubectl get secret -n kubeflare-light-system cloudflare-api-token
```

### CRD Issues

If your custom resources aren't being recognized:

```bash
# Verify CRDs are installed
kubectl get crd | grep cloudflare.io
```

### Common Error Solutions

1. **"ImagePullBackOff" error**: Check your registry credentials and image path
2. **"Unauthorized" error from Cloudflare**: Verify your API token
3. **"Resource not found" error**: Make sure CRDs are installed properly

## Cleaning Up

To remove the operator and all associated resources:

```bash
make undeploy
```

To delete the namespace:

```bash
kubectl delete namespace kubeflare-light-system
```

## Advanced Configuration

### Custom Resource Limits

You can modify the resource limits in the deployment.yaml file:

```yaml
resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 64Mi
```

### High Availability

For production deployments, consider increasing the replica count in deployment.yaml:

```yaml
spec:
  replicas: 2  # Increase for high availability
```

### Custom Metrics

The operator exposes metrics on port 8080. You can configure Prometheus to scrape these metrics for monitoring.