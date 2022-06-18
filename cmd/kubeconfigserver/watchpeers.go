package main

import "time"

type peerNotification struct {
	address string
	added   bool
}

func watchPeers(ch chan<- peerNotification) {
	//list := []peerNotification{{"1.1.1.1", true}, {"2.2.2.2", true}, {"3.3.3.3", true}, {"3.3.3.3", false}}
	list := []peerNotification{{"127.0.0.1", true}}
	for {
		for _, n := range list {
			ch <- n
			time.Sleep(5 * time.Second)
		}
	}
}
