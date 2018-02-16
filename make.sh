#!/bin/bash

echo "Linting..."
gometalinter --skip=vendor --disable=gocyclo ./...

echo "Building..."
mkdir bin || go build -o bin/vxrnet ./docker/vxrnet
