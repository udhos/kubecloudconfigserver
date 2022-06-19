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
	//"github.com/golang/groupcache"
	"github.com/mailgun/groupcache"
	"github.com/udhos/kubecloudconfigserver/env"
	"github.com/udhos/kubecloudconfigserver/refresh"
)

const version = "0.0.0"

type application struct {
	serverMain       *serverGin
	serverHealth     *serverGin
	serverGroupcache *serverHttp
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
	healthAddr := env.String("HEALTH_ADDR", ":3000")
	healthPath := env.String("HEALTH_PATH", "/health")
	groupcachePort := env.String("GROUPCACHE_PORT", ":5000")

	//
	// create backend
	//

	storage := newBackend(backendAddr, backendOptions)

	//
	// create groupcache pool
	//

	myURL := findMyURL(groupcachePort)
	pool := groupcache.NewHTTPPool(myURL)

	//
	// start groupcache server
	//

	app.serverGroupcache = newServerHttp(groupcachePort, pool)

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
		func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			fileName := key
			data, errFetch := storage.fetch(fileName)
			if errFetch != nil {
				log.Printf("fetch: %v", errFetch)
				return errFetch
			}
			dest.SetBytes(data, time.Time{})
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
	app.serverGroupcache.shutdown(timeout)

	log.Print("exiting")
}
