Run these tests against an isolated deployment containing this change:

```sh
go test -tags=integration -v -timeout=60m ./test/integration
```

Set `CICD_PROXY_ACCEPTANCE_CONFIG` to a local JSON configuration file. The suite fails when required configuration is missing. Keep the configuration and authentication files out of version control.

The configuration contains `withLogs` and `withoutLogs` deployment objects. Each has `apiURL` (including the API prefix), `authFile` (a bearer token file), `workspace`, `adminKubeconfig`, and `dataKubeconfig`. The deployments must have OpenSearch enabled and disabled respectively. Empty kubeconfig paths use the usual Kubernetes client configuration.

Also provide `scaleSetRequestFile`, `childRequestFile`, `reachableProxyURL`, `unreachableProxyURL`, and `credentialSecret`. Request files contain valid workload creation requests for the selected workspace, including runner images and resources. The scale set request needs structured GitHub authentication. The referenced general Secret must already be bound to both test workspaces and contain the proxy username and password. The reachable proxy must require that authentication; the unreachable endpoint must refuse connections. Proxy URLs must contain no credentials.

The suite creates its own workloads and deletes only those workloads during cleanup. It retains the pre-existing proxy Secret. The timeout scenarios each take approximately ten minutes. Log enrichment requires the deployment's controller logs to be indexed before the registration deadline.

The schema check can run independently using the current Kubernetes context, or `CICD_PROXY_SCHEMA_KUBECONFIG`:

```sh
go test -tags=integration -v -run '^TestARCStatusContract_FailureMapping$' ./test/integration
```

A matching CRD proxy schema does not establish terminal condition semantics. The optional runner-set failure mapping stays disabled when the installed schema has no conditions, or live terminal behavior has not been confirmed.
