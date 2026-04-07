# Bootstrap Infrastructure Tests

End-to-end tests for Bootstrap components using [Kyverno Chainsaw](https://kyverno.github.io/chainsaw/latest/).

## Prerequisites

Install Chainsaw: https://kyverno.github.io/chainsaw/latest/quick-start/install/

You also need `kubectl` configured with access to the target cluster.

## Running Tests

From the repo root:

```bash
chainsaw test Bootstrap/tests/higress/ --config Bootstrap/tests/.chainsaw.yaml
```

Or with Docker:

```bash
docker run --rm --network=host \
  -v $(pwd)/Bootstrap/tests:/chainsaw \
  -v ${HOME}/.kube:/etc/kubeconfig \
  -e KUBECONFIG=/etc/kubeconfig/config \
  ghcr.io/kyverno/chainsaw:<version> \
  test /chainsaw/higress --config /chainsaw/.chainsaw.yaml
```

## Test Structure

Each Bootstrap component gets its own folder containing a `chainsaw-test.yaml`:

```
tests/
  .chainsaw.yaml          # Shared configuration
  higress/
    chainsaw-test.yaml    # Higress deployment health checks
  README.md
```

## Available Tests

| Test | What it verifies |
|------|------------------|
| `higress/` | higress-controller ready, higress-gateway pods running, Gateway resource exists |
