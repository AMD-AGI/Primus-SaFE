#!/bin/sh
# Build the pod-side reverse-forward multiplexer for every architecture the
# apiserver can inject, and put the results where go:embed picks them up.
#
# The image build runs this before building the apiserver, so the binaries it
# carries are always the ones built from this tree. The tests do not need it: they
# build their own copy for the machine they run on.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
out="$root/apiserver/pkg/handlers/ssh-handlers/muxbin"

for arch in amd64 arm64; do
  # Static and stripped: it is written into a container whose libc is unknown and
  # streamed over an exec for every forward, so size is a latency cost.
  # go build refuses to overwrite the committed placeholder, which is not an
  # object file.
  rm -f "$out/mux-linux-$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w" \
    -o "$out/mux-linux-$arch" "$root/apiserver/cmd/safe-rfwd-mux"
  echo "built $out/mux-linux-$arch"
done
