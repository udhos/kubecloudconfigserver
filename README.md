[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/kubecloudconfigserver/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/kubecloudconfigserver)](https://goreportcard.com/report/github.com/udhos/kubecloudconfigserver)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/kubecloudconfigserver.svg)](https://pkg.go.dev/github.com/udhos/kubecloudconfigserver)

# kubecloudconfigserver

Simple centralized cloud configuration server for Kubernetes in Go.

This configuration server exposes fast cached access to a slow backend.

# Features

- The server is meant to run as multiple replicas service in a Kubernetes cluster.

- [groupcache](https://github.com/golang/groupcache) is used as fast distributed cache. If multiple clients request a new configuration file, only a single replica fetches it from the backend and then makes the file available for all pending requests.

- The pod replicas automatically find each other by querying Kubernetes API for pods with a shared label `app=<app-name>`. For example, if a deployment is used to create the replicas, the shared label would be `app=<deployment-name>`.

- The server handles refresh notification events from the AMQP queue `config-event-queue` below. Whenever a refresh notification is received for an application, cache entries with that application configuration file are cleared, forcing their refresh from the backend.

```
exchangeName: springCloudBus
exchangeType: topic
queue:        config-event-queue
```

- The env var `TTL` can be used to enforce a TTL on cache entries. Example: `TTL=300s`. Default value is `TTL=0`, meaning no expiration set for cache entries.

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
export BACKEND_OPTIONS=flatten ;# flatten request path into a flat dir

kubeconfigserver
```

# Docker

Docker hub:

https://hub.docker.com/r/udhos/kubecloudconfigserver

Pull from docker hub:

```
docker pull udhos/kubecloudconfigserver:0.0.0
```

Build recipe:

```
./docker/build.sh
```
