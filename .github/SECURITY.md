<!-- START OF SECURITY TEMPLATE -->
# Security Policy

## Reporting a Vulnerability

**Do not open a public GitHub issue.** Report privately via one of:

- **GitHub Private Vulnerability Reporting:** [Report a vulnerability](../../security/advisories/new)
- **AMD Product Security portal:** https://www.amd.com/en/resources/product-security.html

Please include: description and impact, steps to reproduce, and affected versions or commits (branch / tag / SHA).

We aim to acknowledge reports within **1 business day** and provide an initial assessment within **5 business days**.

## Scope

This policy covers code and configuration shipped from this repository, including:

- **`SaFE/`** — job-manager, controllers, and Kubernetes platform components
- **`Bootstrap/`** — cluster provisioning and installation scripts
- **`Bench/`** — node health-check and benchmarking suite
- **`Lens/`** — observability stack and metric exporters
- **`Web/`** — frontend dashboards
- **`Scheduler-Plugins/`** — Kubernetes scheduler extensions

The latest commit on `main` is the supported version. We backport security fixes to the most recent tagged release on a best-effort basis.

For issues in third-party dependencies (ROCm, Kubernetes, JuiceFS, Harbor, Higress, Prometheus, Grafana, etc.), please report upstream. For AMD product issues unrelated to this repo, use the [AMD Product Security portal](https://www.amd.com/en/resources/product-security.html).
<!-- END OF SECURITY TEMPLATE -->
