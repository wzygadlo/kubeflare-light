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

## Installation

### 1. Install CRDs
