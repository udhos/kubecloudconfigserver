#!/bin/bash

version=$(go run ./cmd/kubeconfigserver -version | cut -d' ' -f2 | cut -d'=' -f2)

echo kubeconfigserver version=$version

docker build \
    -t udhos/kubecloudconfigserver:$version \
    -t udhos/kubecloudconfigserver:latest \
    -f docker/Dockerfile .
