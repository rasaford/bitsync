package torrent

import (
	"encoding/hex"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/rasaford/bitsync/torrent/file"

	"github.com/nictuku/dht"
	"github.com/rasaford/bitsync/torrent/conf"
	"github.com/rasaford/bitsync/torrent/ldp"
	"github.com/rasaford/bitsync/torrent/listen"
)

func Start(path string, options ...func(*conf.Flags)) {
	path, err := filepath.Abs(path)
	if err != nil {
		log.Printf("cannot get absolute path %v\n", err)
		return
	}
	flags := &conf.Flags{
		Port:                0,
		UseDeadlockDetector: false,
		UseLPD:              true,
		UseUPnP:             true,
		UseNATPMP:           false,
		FileSystemProvider:  file.OsFsProvider{},
		TrackerlessMode:     true,
		Dial:                nil,
		FileDir:             path,
		QuickResume:         true,
		MaxActive:           10,
	}
	for _, option := range options {
		option(flags)
	}
	startTorrent(flags, []string{path})
}

func startTorrent(flags *conf.Flags, torrentFiles []string) error {
	conn, port, err := listen.ListenForPeerConnections(flags)
	if err != nil {
		log.Println("could not listen for peer connection ", err)
	}
	create := make(chan string, flags.MaxActive)
	start := make(chan *TorrentSession, 1)
	done := make(chan *TorrentSession, 1)

	var dhtNode dht.DHT
	if flags.UseDHT {
		dhtNode = *startDHT(flags.Port)
	}
	torrentSessions := make(map[string]*TorrentSession)
	go func() {
		for torrentFile := range create {
			ts, err := NewTorrentSession(flags, torrentFile, uint16(port))
			if err != nil {
				log.Printf("Couldn't create torrent session for %s err: %v\n", torrentFile, err)
				done <- &TorrentSession{}
			} else {
				log.Printf("Created torrent session for %s", ts.M.Info.Name)
				start <- ts
			}
		}
	}()
	torrentQueue := []string{}
	if len(torrentFiles) > flags.MaxActive {
		torrentQueue = torrentFiles[flags.MaxActive:]
	}

	for i, torrentFile := range torrentFiles {
		if i < flags.MaxActive {
			create <- torrentFile
		} else {
			break
		}
	}
	lpdAnnouncer := &ldp.Announcer{}
	if flags.UseLPD {
		lpdAnnouncer, err = ldp.NewAnnouncer(uint16(port))
		if err != nil {
			log.Println("Couldn't listen for Local Peer Discoveries: ", err)
			flags.UseLPD = false
		}
	}

	quit := listenInterrupt()
	theWorldisEnding := false
main:
	for {
		select {
		case ts := <-start:
			if !theWorldisEnding {
				ts.dht = &dhtNode
				if flags.UseLPD {
					lpdAnnouncer.Announce(ts.M.InfoHash)
				}
				torrentSessions[ts.M.InfoHash] = ts
				log.Printf("Starting torrent session for %s", ts.M.Info.Name)
				go func(t *TorrentSession) {
					t.DoTorrent()
					done <- t
				}(ts)
			}
		case ts := <-done:
			if ts.M != nil {
				delete(torrentSessions, ts.M.InfoHash)
				if flags.UseLPD {
					lpdAnnouncer.StopAnnouncing(ts.M.InfoHash)
				}
			}
			if !theWorldisEnding && len(torrentQueue) > 0 {
				create <- torrentQueue[0]
				torrentQueue = torrentQueue[1:]
				continue main
			}

			if len(torrentSessions) == 0 {
				break main
			}
		case <-quit:
			theWorldisEnding = true
			for _, ts := range torrentSessions {
				go ts.Quit()
			}
			log.Print("\nReceived SIGKILL; stopping all torrents\n")
			// TODO: implement graceful shutdown
			os.Exit(1)
		case c := <-conn:
			//	log.Printf("New bt connection for ih %x", c.Infohash)
			if ts, ok := torrentSessions[c.Infohash]; ok {
				ts.AcceptNewPeer(c)
			}
		case dhtPeers := <-dhtNode.PeersRequestResults:
			for key, peers := range dhtPeers {
				if ts, ok := torrentSessions[string(key)]; ok {
					// log.Printf("Received %d DHT peers for torrent session %x\n", len(peers), []byte(key))
					for _, peer := range peers {
						peer = dht.DecodePeerAddress(peer)
						ts.HintNewPeer(peer)
					}
				} else {
					log.Printf("Received DHT peer for an unknown torrent session %x\n", []byte(key))
				}
			}
		case announce := <-lpdAnnouncer.Announces:
			hexhash, err := hex.DecodeString(announce.Infohash)
			if err != nil {
				log.Println("Err with hex-decoding:", err)
			}
			if ts, ok := torrentSessions[string(hexhash)]; ok {
				// log.Printf("Received LPD announce for ih %s", announce.Infohash)
				ts.HintNewPeer(announce.Peer)
			}
		}
	}
	if flags.UseDHT {
		dhtNode.Stop()
	}
	return err
}

func startDHT(listenPort int) *dht.DHT {
	// TODO: UPnP UDP port mapping.
	cfg := dht.NewConfig()
	cfg.Port = listenPort
	cfg.NumTargetPeers = conf.TARGET_NUM_PEERS
	dhtnode, err := dht.New(cfg)
	if err != nil {
		log.Println("DHT node creation error:", err)
		return nil
	}
	go dhtnode.Run()
	return dhtnode
}

func listenInterrupt() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	return c
}
