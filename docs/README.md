# 📚 Kubeflare Light Documentation

Welcome to the comprehensive documentation for kubeflare-light, a Kubernetes controller for managing Cloudflare security resources.

## 📖 Documentation Index

### Getting Started
- [📋 Main README](../README.md) - Project overview and quick start
- [🚀 Installation Guide](../DEPLOYMENT.md) - Detailed installation instructions
- [⚡ Quick Start Examples](../examples/) - Ready-to-use example configurations

### User Guides
- [📚 API Reference](./api-reference.md) - Complete CRD specification and examples
- [🎯 Usage Examples](./usage-examples.md) - Common use cases and patterns
- [🔧 Configuration Guide](./configuration.md) - Advanced configuration options
- [🛠️ Troubleshooting](./troubleshooting.md) - Common issues and solutions

### Developer Documentation
- [👨‍💻 Developer Guide](./developer-guide.md) - Architecture and development setup
- [🏗️ Contributing Guidelines](./contributing.md) - How to contribute to the project
- [🧪 Testing Guide](./testing.md) - Testing strategies and frameworks
- [📦 Release Process](./release-process.md) - Version management and releases

### Operations & Security
- [🛡️ Security Guide](./security.md) - Security best practices and vulnerability information
- [📊 Monitoring & Observability](./monitoring.md) - Metrics, logging, and alerting
- [🔄 Backup & Recovery](./backup-recovery.md) - Data protection strategies
- [🎛️ Production Deployment](./production-deployment.md) - Production readiness checklist

### Advanced Topics
- [🌐 Multi-Zone Management](./multi-zone.md) - Managing multiple Cloudflare zones
- [🔌 Webhook Configuration](./webhooks.md) - Admission controllers and validation
- [📈 Performance Tuning](./performance.md) - Optimization strategies
- [🔄 Migration Guide](./migration.md) - Migrating from other solutions

### Reference
- [📋 CLI Reference](./cli-reference.md) - Command-line interface documentation
- [🔧 Helm Chart Reference](./helm-reference.md) - Helm chart configuration options
- [📝 Changelog](../CHANGELOG.md) - Version history and changes
- [❓ FAQ](./faq.md) - Frequently asked questions

## 🏛️ Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                       │
│                                                             │
│  ┌─────────────────┐    ┌──────────────────────────────┐   │
│  │   User/GitOps   │    │        kubeflare-light       │   │
│  │   kubectl       │───▶│         Controller           │   │
│  │   apply CRDs    │    │                              │   │
│  └─────────────────┘    │  ┌────────────────────────┐  │   │
│                         │  │    RateLimit           │  │   │
│  ┌─────────────────┐    │  │    Controller          │  │   │
│  │   Custom        │◀───┤  │                        │  │   │
│  │   Resources     │    │  └────────────────────────┘  │   │
│  │   (CRDs)        │    │                              │   │
│  └─────────────────┘    │  ┌────────────────────────┐  │   │
│                         │  │    WAFRule             │  │   │
│  ┌─────────────────┐    │  │    Controller          │  │   │
│  │   ConfigMaps    │───▶│  │                        │  │   │
│  │   Secrets       │    │  └────────────────────────┘  │   │
│  └─────────────────┘    └──────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                                     │
                                     │ Cloudflare Go SDK v4.5.1
                                     │ (Rulesets API)
                                     ▼
┌─────────────────────────────────────────────────────────────┐
│                     Cloudflare API                          │
│  ┌──────────────────┐      ┌──────────────────────────┐    │
│  │   Rate Limiting  │      │      WAF Rules           │    │
│  │   (Rulesets API) │      │   (Custom Rulesets)      │    │
│  └──────────────────┘      └──────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## 🎯 Key Features

### ✨ Core Capabilities
- **Rate Limit Management**: Create and manage Cloudflare rate limiting rules
- **WAF Rule Management**: Configure Web Application Firewall rules
- **GitOps Ready**: Version control your security policies
- **Kubernetes Native**: Full integration with K8s RBAC, events, and monitoring

### 🔧 Advanced Features
- **Zone Resolution**: Flexible zone ID management via ConfigMaps
- **Helm Charts**: Production-ready deployment templates
- **Expression Validation**: Cloudflare expression syntax validation
- **Multi-Tenant**: Namespace-based isolation and RBAC

### 🛡️ Security & Compliance
- **Input Validation**: Comprehensive input sanitization
- **Audit Logging**: Complete audit trail of all operations
- **Secret Management**: Secure handling of API credentials
- **RBAC Integration**: Fine-grained access control

## 🚦 Project Status

| Component | Status | Version | Notes |
|-----------|--------|---------|-------|
| **Rate Limiting** | ✅ Stable | v1alpha1 | Production ready |
| **WAF Rules** | ⚠️ Beta | v1alpha1 | Implementation in progress |
| **Zone Resolution** | ✅ Stable | v1 | Production ready |
| **Helm Charts** | ✅ Stable | v1 | Production ready |
| **Documentation** | ✅ Complete | v1 | Comprehensive guides |

### 🔒 Security Status
- **Expression Injection**: ⚠️ **Known vulnerability** - patch in progress
- **Input Validation**: ✅ Implemented for most fields
- **RBAC**: ✅ Complete implementation
- **Secrets**: ✅ Secure management practices

## 🛣️ Roadmap

### Short Term (Q1 2025)
- [ ] Fix expression injection vulnerability
- [ ] Complete WAF rule implementation
- [ ] Add admission webhooks
- [ ] Performance optimizations

### Medium Term (Q2 2025)
- [ ] Multi-zone management UI
- [ ] Advanced expression builder
- [ ] Prometheus metrics integration
- [ ] Backup/restore functionality

### Long Term (Q3-Q4 2025)
- [ ] Policy-as-code framework
- [ ] Advanced analytics
- [ ] Integration with other security tools
- [ ] Cloud provider integrations

## 🤝 Community & Support

### Getting Help
- **Documentation**: Start with this documentation
- **GitHub Issues**: [Report bugs and request features](https://github.com/wzygadlo/kubeflare-light/issues)
- **Discussions**: [Community discussions](https://github.com/wzygadlo/kubeflare-light/discussions)

### Contributing
We welcome contributions! Please read our [Contributing Guide](./contributing.md) for details on:
- Code of conduct
- Development setup
- Pull request process
- Testing requirements

### Communication Channels
- **GitHub**: Primary development and issue tracking
- **Documentation**: Always up-to-date guides and references
- **Security**: Responsible disclosure process

## 📄 License & Legal

- **License**: MIT License - see [LICENSE](../LICENSE) file
- **Copyright**: 2025 kubeflare-light contributors
- **Trademark**: Cloudflare is a trademark of Cloudflare, Inc.

---

## 🔍 Quick Navigation

### For Users
1. [Install kubeflare-light](../README.md#quick-start)
2. [Configure your first rate limit](./api-reference.md#ratelimit-api)
3. [Set up zone resolution](./api-reference.md#zone-resolution)
4. [Monitor your deployment](./monitoring.md)

### For Developers
1. [Set up development environment](./developer-guide.md#development-setup)
2. [Understand the architecture](./developer-guide.md#architecture-overview)
3. [Write your first test](./testing.md)
4. [Submit a pull request](./contributing.md)

### For Operators
1. [Deploy to production](./production-deployment.md)
2. [Configure monitoring](./monitoring.md)
3. [Set up security policies](./security.md)
4. [Plan backup strategy](./backup-recovery.md)

---

**Need something specific?** Use the search function or check our [FAQ](./faq.md) for common questions.