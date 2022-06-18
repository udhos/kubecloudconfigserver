package main

import (
	"context"
	"log"
	"net"
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
	"golang.org/x/exp/maps"
)

const version = "0.0.0"

type application struct {
	serverMain *server
	me         string
}

func main() {

	app := &application{
		me: filepath.Base(os.Args[0]),
	}

	log.Printf("%s version=%s runtime=%s GOOS=%s GOARCH=%s GOMAXPROCS=%d",
		app.me, version, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))

	log.Printf("examples: export BACKEND=http://configserver:9000")
	log.Printf("          export BACKEND=dir:samples")

	applicationAddr := env.String("LISTEN_ADDR", ":8080")
	backendAddr := env.String("BACKEND", "dir:samples")

	storage := newBackend(backendAddr)

	myURL := findMyURL(applicationAddr)

	pool := groupcache.NewHTTPPool(myURL)

	go updatePeers(pool, myURL, applicationAddr)

	app.serverMain = newServer(applicationAddr)

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

	go func() {
		log.Printf("application server: listening on %s", applicationAddr)
		err := app.serverMain.server.ListenAndServe()
		log.Printf("application server: exited: %v", err)
	}()

	shutdown(app)
}

func shutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("received signal '%v', initiating shutdown", sig)

	const timeout = 5 * time.Second
	app.serverMain.shutdown(timeout)

	log.Print("exiting")
}

func findMyURL(port string) string {
	host, errHost := os.Hostname()
	if errHost != nil {
		log.Fatalf("findMyURL: hostname '%s': %v", host, errHost)
	}
	addrs, errAddr := net.LookupHost(host)
	if errAddr != nil {
		log.Fatalf("findMyURL: hostname '%s' lookup: %v", host, errAddr)
	}
	if len(addrs) < 1 {
		log.Fatalf("findMyURL: hostname '%s': no addr found", host)
	}
	if len(addrs) > 1 {
		log.Printf("findMyURL: hostname '%s': found multiple addresses: %v", host, addrs)
	}
	url := buildURL(addrs[0], port)
	log.Printf("findMyURL: found: %s", url)
	return url
}

func buildURL(addr, port string) string {
	return "http://" + addr + port
}

func updatePeers(pool *groupcache.HTTPPool, myURL, port string) {

	peers := map[string]bool{
		myURL: true,
	}

	keys := maps.Keys(peers)
	log.Printf("updatePeers: initial peers: %v", keys)
	pool.Set(keys...)

	ch := make(chan peerNotification)

	go watchPeers(ch)

	for n := range ch {
		url := buildURL(n.address, port)
		log.Printf("updatePeers: peer=%s added=%t", url, n.added)
		if n.added {
			peers[url] = true
		} else {
			delete(peers, url)
		}
		keys := maps.Keys(peers)
		log.Printf("updatePeers: current peers: %v", keys)
		pool.Set(keys...)
	}

	log.Fatal("updatePeers: closed channel")
}
