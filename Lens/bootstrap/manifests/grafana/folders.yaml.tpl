apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaFolder
metadata:
  name: kubernetes
spec:
  instanceSelector:
    matchLabels:
      system: primus-lens
  title: "Kubernetes"
  permissions: |
    {
      "items": [
        {
          "role": "Admin",
          "permission": 4
        },
        {
          "role": "Editor",
          "permission": 2
        },
        {
          "role": "Viewer",
          "permission": 1
        }
      ]
    }
---
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaFolder
metadata:
  name: middleware
spec:
  instanceSelector:
    matchLabels:
      system: primus-lens
  title: "Middleware"
  permissions: |
    {
      "items": [
        {
          "role": "Admin",
          "permission": 4
        },
        {
          "role": "Editor",
          "permission": 2
        },
        {
          "role": "Viewer",
          "permission": 1
        }
      ]
    }
---
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaFolder
metadata:
  name: node
spec:
  instanceSelector:
    matchLabels:
      system: primus-lens
  title: "Node"
  permissions: |
    {
      "items": [
        {
          "role": "Admin",
          "permission": 4
        },
        {
          "role": "Editor",
          "permission": 2
        },
        {
          "role": "Viewer",
          "permission": 1
        }
      ]
    }
---

