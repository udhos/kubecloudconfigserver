#!/bin/bash

app=kubeconfigserver

version=$(go run ./cmd/$app -version | cut -d' ' -f2 | cut -d'=' -f2)

echo $app version=$version

docker build \
    -t udhos/$app:$version \
    -t udhos/$app:latest \
    -f docker/Dockerfile .

echo "push: docker push udhos/$app:$version; docker push udhos/$app:latest"
