# kubecloudconfigserver

Simple centralized cloud configuration server for Kubernetes in Go.

This configuration server exposes fast cached access to a slow backend.

# Features

- The server is meant to run as multiple replicas service in a Kubernetes cluster.

- [groupcache](https://github.com/golang/groupcache) is used as fast distributed cache. If multiple clients request a new configuration file, only a single replica fetches it from the backend and then makes the file available for all pending requests.

- The pod replicas automatically find each other by querying Kubernetes API for pods with a shared label `app=<app-name>`. For example, if a deployment is used to create the replicas, the shared label would be `app=<deployment-name>`.

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
