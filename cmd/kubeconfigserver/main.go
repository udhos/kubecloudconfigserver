package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/groupcache"
	"github.com/udhos/kubecloudconfigserver/env"
	"github.com/udhos/kubecloudconfigserver/refresh"
)

const version = "0.0.0"

type application struct {
	serverMain   *server
	serverHealth *server
	me           string
}

func main() {

	app := &application{
		me: filepath.Base(os.Args[0]),
	}

	log.Printf("%s version=%s runtime=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		app.me, version, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))

	debug := env.Bool("DEBUG", true)

	log.Printf("backend http:                     export BACKEND=http://configserver:9000")
	log.Printf("backend directory:                export BACKEND=dir:samples")
	log.Printf("backend directory option flatten: export BACKEND_OPTIONS=flatten")

	applicationAddr := env.String("LISTEN_ADDR", ":8080")
	backendAddr := env.String("BACKEND", "dir:samples")
	backendOptions := env.String("BACKEND_OPTIONS", "")
	refreshAmqpURL := env.String("AMQP_URL", "amqp://guest:guest@rabbitmq:5672/")
	refreshEnabled := env.Bool("REFRESH", false)
	healthAddr := env.String("HEALTH_ADDR", ":3000")
	healthPath := env.String("HEALTH_PATH", "/health")

	//
	// create backend
	//

	storage := newBackend(backendAddr, backendOptions)

	//
	// create groupcache pool
	//

	myURL := findMyURL()
	pool := groupcache.NewHTTPPool(myURL)

	//
	// start watcher for addresses of peers
	//

	go updatePeers(pool)

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	configFiles := groupcache.NewGroup("configfiles", 64<<20, groupcache.GetterFunc(
		func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			fileName := key
			data, errFetch := storage.fetch(fileName)
			if errFetch != nil {
				log.Printf("fetch: %v", errFetch)
				return errFetch
			}
			dest.SetBytes(data)
			return nil
		}))

	//
	// receive refresh events
	//

	if refreshEnabled {
		// "#" means all applications
		refresher := refresh.New(refreshAmqpURL, app.me, []string{"#"}, debug, nil)

		go func() {
			for app := range refresher.C {
				log.Printf("received refresh for application='%s'", app)
				log.Printf("FIXME WRITEME remove from cache application='%s'", app)
			}

			log.Fatal("refresh channel has been closed")
		}()
	}

	//
	// register application routes
	//

	app.serverMain = newServer(applicationAddr)

	const pathAny = "/*anything"
	log.Printf("registering route: %s %s", applicationAddr, pathAny)
	app.serverMain.router.GET(pathAny, func(c *gin.Context) {
		path := c.Param("anything")
		log.Printf("path: '%s'", path)

		var data []byte
		if errGet := configFiles.Get(context.TODO(), path, groupcache.AllocatingByteSliceSink(&data)); errGet != nil {
			log.Printf("groupcache.Get: %v", errGet)
			c.String(http.StatusInternalServerError, "server error")
			return
		}

		log.Printf("stats main: %#v", configFiles.CacheStats(groupcache.MainCache))
		log.Printf("stats hot:  %#v", configFiles.CacheStats(groupcache.MainCache))

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

	app.serverHealth = newServer(healthAddr)

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
	// handle graceful shutdown
	//

	shutdown(app)
}

func shutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("received signal '%v', initiating shutdown", sig)

	const timeout = 5 * time.Second
	app.serverHealth.shutdown(timeout)
	app.serverMain.shutdown(timeout)

	log.Print("exiting")
}
