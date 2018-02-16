#!/bin/bash

echo "Linting..."
gometalinter --skip=vendor --disable=gocyclo ./...

echo "Building..."
mkdir bin 2>/dev/null || true
go build -o bin/vxrnet ./docker/vxrnet
