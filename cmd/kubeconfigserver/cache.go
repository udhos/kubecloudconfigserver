package main

import (
	"log"
	"net"
	"os"

	//"github.com/golang/groupcache"
	"github.com/mailgun/groupcache"
	"golang.org/x/exp/maps"
)

func findMyURL(groupcachePort string) string {
	url := buildURL(findMyAddr(), groupcachePort)
	log.Printf("findMyURL: found: %s", url)
	return url
}

func findMyAddr() string {
	host, errHost := os.Hostname()
	if errHost != nil {
		log.Fatalf("findMyAddr: hostname '%s': %v", host, errHost)
	}
	addrs, errAddr := net.LookupHost(host)
	if errAddr != nil {
		log.Fatalf("findMyAddr: hostname '%s' lookup: %v", host, errAddr)
	}
	if len(addrs) < 1 {
		log.Fatalf("findMyAddr: hostname '%s': no addr found", host)
	}
	if len(addrs) > 1 {
		log.Printf("findMyAddr: hostname '%s': found multiple addresses: %v", host, addrs)
	}
	addr := addrs[0]
	log.Printf("findMyAddr: found: %s", addr)
	return addr
}

func buildURL(addr, groupcachePort string) string {
	return "http://" + addr + groupcachePort
}

func updatePeers(pool *groupcache.HTTPPool, groupcachePort string) {

	kc, errClient := newKubeClient()
	if errClient != nil {
		log.Fatalf("updatePeers: kube client: %v", errClient)
	}

	addresses, errList := kc.listPodsAddresses()
	if errList != nil {
		log.Fatalf("updatePeers: list addresses: %v", errList)
	}

	addresses = append(addresses, findMyAddr()) // force my own addr

	peers := map[string]bool{}

	for _, addr := range addresses {
		url := buildURL(addr, groupcachePort)
		peers[url] = true
	}

	keys := maps.Keys(peers)
	log.Printf("updatePeers: initial peers: %v", keys)
	pool.Set(keys...)

	ch := make(chan podAddress)

	go watchPeers(kc, ch)

	for n := range ch {
		url := buildURL(n.address, groupcachePort)
		log.Printf("updatePeers: peer=%s added=%t", url, n.added)
		count := len(peers)
		if n.added {
			peers[url] = true
		} else {
			delete(peers, url)
		}
		if len(peers) == count {
			continue
		}
		keys := maps.Keys(peers)
		log.Printf("updatePeers: current peers: %v", keys)
		pool.Set(keys...)
	}

	log.Printf("updatePeers: channel has been closed, nothing to do, exiting")
}

func watchPeers(kc kubeClient, ch chan<- podAddress) {
	errWatch := kc.watchPodsAddresses(ch)
	if errWatch != nil {
		log.Fatalf("watchPeers: %v", errWatch)
	}
	log.Printf("watchPeers: nothing to do, exiting")
}
