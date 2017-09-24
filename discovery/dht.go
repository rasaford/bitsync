package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/nictuku/dht"
)

const (
	port      = 4711
	numTarget = 10
)

func main() {
	hash := hashSha1(time.Now().String())
	fmt.Println(hash)
	ih, err := dht.DecodeInfoHash(hash)
	if err != nil {
		log.Fatal(err)
	}
	d, err := dht.New(nil)
	if err != nil {
		log.Fatal(err)
	}
	if err = d.Start(); err != nil {
		log.Fatal(err)
	}

	log.Println("started dht client")
	go drainResults(d)
	for {
		d.PeersRequest(string(ih), false)
		time.Sleep(5 * time.Second)
	}
}

func drainResults(n *dht.DHT) {
	count := 0
	for r := range n.PeersRequestResults {
		for _, peers := range r {
			for _, peer := range peers {
				log.Printf("%d: %v\n", count, dht.DecodePeerAddress(peer))
				count++
			}
		}
	}
}

func hashSha1(input string) string {
	hash := sha1.New()
	hash.Write([]byte(input))
	res := hash.Sum(nil)
	dest := make([]byte, hex.EncodedLen(len(res)))
	hex.Encode(dest, res)
	return string(dest)
}
