#!/bin/bash

echo "Linting..."
gometalinter --skip=vendor --disable=gocyclo ./...

echo "Building..."
go build -o vxrnet ./docker
