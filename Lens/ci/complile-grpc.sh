#!/bin/bash
cd modules/core/ && protoc \
  --go_out=. \
  --go-grpc_out=. \
  --go_opt=paths=source_relative \
  --go-grpc_opt=paths=source_relative \
  pkg/pb/exporter/exporter.proto
