# Bootstrap Infrastructure Tests

Infrastructure unit tests for Bootstrap components using [Kyverno Chainsaw](https://kyverno.github.io/chainsaw/latest/).

## Prerequisites

Quickstart Chainsaw: https://kyverno.github.io/chainsaw/latest/quick-start/install/

You also need `kubectl` configured with access to the target cluster.

## Running Tests

From the `Bootstrap` directory:

```bash
cd ~/Primus-SaFE/Bootstrap
sudo nerdctl run --rm \
    -v ./tests/:/chainsaw/ \
    -v ${HOME}/.kube/:/etc/kubeconfig/ \
    -e KUBECONFIG=/etc/kubeconfig/config \
    --network=host \
    ghcr.io/kyverno/chainsaw \
    test /chainsaw --config /chainsaw/.chainsaw.yaml
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
