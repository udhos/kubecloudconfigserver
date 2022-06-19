# kubecloudconfigserver

Simple centralized cloud configuration server for Kubernetes in Go.

This configuration server exposes fast cached access to a slow backend.

# Build

```
git clone https://github.com/udhos/kubecloudconfigserver

cd kubecloudconfigserver

go mod tidy

go test -race ./...

export CGO_ENABLED=0

go install ./...
```

# Run

## HTTP backend

Proxying to an HTTP server:

```
export BACKEND=http://configserver:9000
kubeconfigserver
```

## Directory backend

Reading from filesystem rooted at directory `samples`:

```
export BACKEND=dir:samples
BACKEND_OPTIONS=flatten    ;# flatten request path into a flat dir
kubeconfigserver
```
