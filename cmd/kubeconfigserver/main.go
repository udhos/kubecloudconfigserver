package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	//"github.com/golang/groupcache"
	"github.com/mailgun/groupcache"
	"github.com/udhos/kubecloudconfigserver/env"
	"github.com/udhos/kubecloudconfigserver/refresh"
)

const version = "0.0.1"

type application struct {
	serverMain       *serverGin
	serverHealth     *serverGin
	serverMetrics    *serverGin
	serverGroupcache *serverHTTP
	me               string
}

func getVersion(me string) string {
	return fmt.Sprintf("%s version=%s runtime=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		me, version, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
}

func main() {

	app := &application{
		me: filepath.Base(os.Args[0]),
	}

	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	{
		v := getVersion(app.me)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Print(v)
	}

	log.Printf("configuration hints:")
	log.Printf("backend http:                     export BACKEND=http://configserver:9000")
	log.Printf("backend directory:                export BACKEND=dir:samples")
	log.Printf("backend directory option flatten: export BACKEND_OPTIONS=flatten")
	log.Printf("disable refresh:                  export REFRESH=false")

	debug := env.Bool("DEBUG", true)
	applicationAddr := env.String("LISTEN_ADDR", ":8080")
	backendAddr := env.String("BACKEND", "dir:samples")
	backendOptions := env.String("BACKEND_OPTIONS", "")
	refreshAmqpURL := env.String("AMQP_URL", "amqp://guest:guest@rabbitmq:5672/")
	refreshEnabled := env.Bool("REFRESH", true)
	healthAddr := env.String("HEALTH_ADDR", ":8888")
	healthPath := env.String("HEALTH_PATH", "/health")
	metricsAddr := env.String("METRICS_ADDR", ":3000")
	metricsPath := env.String("METRICS_PATH", "/metrics")
	groupcachePort := env.String("GROUPCACHE_PORT", ":5000")
	ttl := env.Duration("TTL", time.Duration(0))
	jaegerURL := env.String("JAEGER_URL", "http://jaeger-collector:14268/api/traces")
	cache := env.Bool("CACHE", true)

	//
	// initialize tracing
	//

	var tracer trace.Tracer

	{
		tp, errTracer := tracerProvider(app.me, jaegerURL)
		if errTracer != nil {
			log.Fatal(errTracer)
		}

		// Register our TracerProvider as the global so any imported
		// instrumentation in the future will default to using it.
		otel.SetTracerProvider(tp)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Cleanly shutdown and flush telemetry when the application exits.
		defer func(ctx context.Context) {
			// Do not make the application hang when it is shutdown.
			ctx, cancel = context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Fatal(err)
			}
		}(ctx)

		tracePropagation()

		tracer = tp.Tracer("component-main")
	}

	//
	// create backend
	//

	storage := newBackend(tracer, backendAddr, backendOptions)

	//
	// create groupcache pool
	//

	myURL := findMyURL(groupcachePort)
	pool := groupcache.NewHTTPPool(myURL)

	//
	// start groupcache server
	//

	app.serverGroupcache = newServerHTTP(groupcachePort, pool)

	go func() {
		log.Printf("groupcache server: listening on %s", groupcachePort)
		err := app.serverGroupcache.server.ListenAndServe()
		log.Printf("groupcache server: exited: %v", err)
	}()

	//
	// start watcher for addresses of peers
	//

	go updatePeers(pool, groupcachePort)

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	configFiles := groupcache.NewGroup("configfiles", 64<<20, groupcache.GetterFunc(
		func(ctx groupcache.Context, filename string, dest groupcache.Sink) error {
			/*
				begin := time.Now()
				data, errFetch := storage.fetch(ctx.(context.Context), filename)
				if errFetch != nil {
					log.Printf("fetch: filename='%s': error: %v", filename, errFetch)
					return errFetch
				}
				elap := time.Since(begin)
				log.Printf("fetch: filename='%s': size:%d elapsed:%v", filename, len(data), elap)
			*/

			var newCtx context.Context
			if ctx == nil {
				newCtx = context.Background()
			} else {
				newCtx = ctx.(context.Context)
			}

			data, errFetch := fetch(newCtx, storage, filename)
			if errFetch != nil {
				return errFetch
			}
			var expire time.Time // zero value for expire means no expiration
			if ttl != 0 {
				expire = time.Now().Add(ttl)
			}
			dest.SetBytes(data, expire)
			return nil
		}))

	// tableKeys is used to find which key should be removed for a refresh notification
	// Example:
	// received notification for: application=config2
	// then should remove cache key: /path/to/config1-default.yml,config2-default.yml,config3-default.yml
	tableKeys := newTable()

	//
	// receive refresh events
	//

	if refreshEnabled {
		// "#" means receive notification for all applications
		refresher := refresh.New(refreshAmqpURL, app.me, []string{"#"}, debug, nil)

		go func() {
			for app := range refresher.C {
				// app = "config-cli-example:**"
				log.Printf("refresh: received notification for application='%s'", app)
				for _, key := range tableKeys.match(app) {
					log.Printf("refresh: removing key='%s' for application='%s'", key, app)
					if errRemove := configFiles.Remove(context.TODO(), key); errRemove != nil {
						log.Printf("refresh: removing key='%s' for application='%s': error: %v",
							key, app, errRemove)
						continue
					}
					tableKeys.del(key)
				}
			}

			log.Fatal("refresh channel has been closed")
		}()
	}

	//
	// register application routes
	//

	app.serverMain = newServerGin(applicationAddr)
	app.serverMain.router.Use(metricsMiddleware())
	app.serverMain.router.Use(gin.Logger())
	app.serverMain.router.Use(otelgin.Middleware(app.me))

	const pathAny = "/*anything"
	log.Printf("registering route: %s %s", applicationAddr, pathAny)
	app.serverMain.router.GET(pathAny, func(c *gin.Context) {

		path := c.Param("anything")

		ctx := c.Request.Context()

		newCtx, span := tracer.Start(ctx, "request")
		defer span.End()

		log.Printf("traceID=%s", span.SpanContext().TraceID())

		if !cache {
			// cache disabled
			data, errFetch := fetch(ctx, storage, path)
			if errFetch != nil {
				span.SetStatus(codes.Error, errFetch.Error())
				c.String(http.StatusInternalServerError, "fetch error")
				return
			}

			c.Data(http.StatusOK, http.DetectContentType(data), data)
			return
		}

		var data []byte
		errGet := configFiles.Get(newCtx, path, groupcache.AllocatingByteSliceSink(&data))
		log.Printf("groupcache.Get: path='%s' error:%v", path, errGet)
		if errGet != nil {
			if errBackend, isBackend := errGet.(backendError); isBackend {
				span.SetStatus(codes.Error, errBackend.Error())
				sendBackendError(c, errBackend)
				return
			}
			span.SetStatus(codes.Error, errGet.Error())
			c.String(http.StatusInternalServerError, "server error")
			return
		}

		tableKeys.add(path) // record path

		log.Printf("stats main: %#v", configFiles.CacheStats(groupcache.MainCache))
		log.Printf("stats hot:  %#v", configFiles.CacheStats(groupcache.HotCache))

		c.Data(http.StatusOK, http.DetectContentType(data), data)
	})

	//
	// start application server
	//

	go func() {
		log.Printf("application server: listening on %s", applicationAddr)
		err := app.serverMain.server.ListenAndServe()
		log.Printf("application server: exited: %v", err)
	}()

	//
	// start health server
	//

	app.serverHealth = newServerGin(healthAddr)

	log.Printf("registering route: %s %s", healthAddr, healthPath)
	app.serverHealth.router.GET(healthPath, func(c *gin.Context) {
		c.String(http.StatusOK, "health ok")
	})

	go func() {
		log.Printf("health server: listening on %s", healthAddr)
		err := app.serverHealth.server.ListenAndServe()
		log.Printf("health server: exited: %v", err)
	}()

	//
	// start metrics server
	//

	app.serverMetrics = newServerGin(metricsAddr)

	go func() {
		prom := promhttp.Handler()
		app.serverMetrics.router.GET(metricsPath, func(c *gin.Context) {
			prom.ServeHTTP(c.Writer, c.Request)
		})
		log.Printf("metrics server: listening on %s %s", metricsAddr, metricsPath)
		err := app.serverMetrics.server.ListenAndServe()
		log.Printf("metrics server: exited: %v", err)
	}()

	//
	// handle graceful shutdown
	//

	shutdown(app)
}

func fetch(ctx context.Context, storage backend, filename string) ([]byte, error) {
	begin := time.Now()
	data, errFetch := storage.fetch(ctx, filename)
	if errFetch != nil {
		log.Printf("fetch: filename='%s': error: %v", filename, errFetch)
		return data, errFetch
	}
	elap := time.Since(begin)
	log.Printf("fetch: filename='%s': size:%d elapsed:%v", filename, len(data), elap)
	return data, nil
}

func shutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("received signal '%v', initiating shutdown", sig)

	const timeout = 5 * time.Second
	app.serverHealth.shutdown(timeout)
	app.serverMetrics.shutdown(timeout)
	app.serverMain.shutdown(timeout)
	app.serverGroupcache.shutdown(timeout)

	log.Print("exiting")
}
