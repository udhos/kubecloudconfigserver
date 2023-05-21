package main

import (
	"time"

	"github.com/udhos/boilerplate/envconfig"
)

type appConfig struct {
	debug           bool
	applicationAddr string
	backendAddr     string
	backendOptions  string
	refreshAmqpURL  string
	refreshEnabled  bool
	healthAddr      string
	healthPath      string
	metricsAddr     string
	metricsPath     string
	groupcachePort  string
	ttl             time.Duration
	jaegerURL       string
	cache           bool
}

func newConfig(roleSessionName string) appConfig {

	env := envconfig.NewSimple(roleSessionName)

	return appConfig{
		debug:           env.Bool("DEBUG", true),
		applicationAddr: env.String("LISTEN_ADDR", ":8080"),
		backendAddr:     env.String("BACKEND", "dir:samples"),
		backendOptions:  env.String("BACKEND_OPTIONS", ""),
		refreshAmqpURL:  env.String("AMQP_URL", "amqp://guest:guest@rabbitmq:5672/"),
		refreshEnabled:  env.Bool("REFRESH", true),
		healthAddr:      env.String("HEALTH_ADDR", ":8888"),
		healthPath:      env.String("HEALTH_PATH", "/health"),
		metricsAddr:     env.String("METRICS_ADDR", ":3000"),
		metricsPath:     env.String("METRICS_PATH", "/metrics"),
		groupcachePort:  env.String("GROUPCACHE_PORT", ":5000"),
		ttl:             env.Duration("TTL", time.Duration(0)),
		jaegerURL:       env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces"),
		cache:           env.Bool("CACHE", true),
	}
}
