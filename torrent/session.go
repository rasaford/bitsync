package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rasaford/bitsync/defaults"

	bencode "github.com/jackpal/bencode-go"
	"github.com/nictuku/dht"
	"github.com/nictuku/nettools"
	"github.com/rasaford/bitsync/torrent/bitset"
	"github.com/rasaford/bitsync/torrent/choker"
	"github.com/rasaford/bitsync/torrent/conf"
	"github.com/rasaford/bitsync/torrent/convert"
	"github.com/rasaford/bitsync/torrent/file"
	"github.com/rasaford/bitsync/torrent/listen"
	"github.com/rasaford/bitsync/torrent/meta"
	"github.com/rasaford/bitsync/torrent/peer"
	"github.com/rasaford/bitsync/torrent/proxy"
	"github.com/rasaford/bitsync/torrent/trackerClient"
)

func peerID() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	sid := "-tt" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(r.Int63(), 10)
	return sid[0:20]
}

var kBitTorrentHeader = []byte{'\x13', 'B', 'i', 't', 'T', 'o', 'r',
	'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}

type TorrentSession struct {
	flags                *conf.Flags
	M                    *meta.MetaInfo
	Session              meta.SessionInfo
	ti                   *meta.TrackerResponse
	torrentHeader        []byte
	fileStore            file.FileStore
	trackerReportChan    chan trackerClient.ClientStatusReport
	trackerInfoChan      chan *meta.TrackerResponse
	hintNewPeerChan      chan string
	addPeerChan          chan *listen.BtConn
	peers                map[string]*peer.PeerState
	peerMessageChan      chan peer.PeerMessage
	pieceSet             *bitset.Bitset // The pieces we have
	totalPieces          int
	totalSize            int64
	lastPieceLength      int
	goodPieces           int
	activePieces         map[int]*ActivePiece
	maxActivePieces      int
	heartbeat            chan bool
	dht                  *dht.DHT
	quit                 chan bool
	ended                chan bool
	trackerLessMode      bool
	torrentFile          string
	chokePolicy          choker.ChokePolicy
	chokePolicyHeartbeat <-chan time.Time
	execOnSeedingDone    bool
}

func NewTorrentSession(flags *conf.Flags, torrent string, listenPort uint16) (t *TorrentSession, err error) {
	ts := &TorrentSession{
		flags:                flags,
		peers:                make(map[string]*peer.PeerState),
		peerMessageChan:      make(chan peer.PeerMessage),
		activePieces:         make(map[int]*ActivePiece),
		quit:                 make(chan bool),
		ended:                make(chan bool),
		torrentFile:          torrent,
		chokePolicy:          &choker.ClassicChokePolicy{},
		chokePolicyHeartbeat: time.Tick(10 * time.Second),
		execOnSeedingDone:    len(flags.ExecOnSeeding) == 0,
	}
	fromMagnet := strings.HasPrefix(torrent, "magnet:")
	ts.M, err = meta.GetMetaInfo(flags.Dial, torrent)
	if err != nil {
		return
	}

	if ts.M.Announce == "" && len(ts.M.AnnounceList) == 0 {
		ts.trackerLessMode = true
	} else {
		ts.trackerLessMode = ts.flags.TrackerlessMode
	}

	dhtAllowed := flags.UseDHT && ts.M.Info.Private == 0
	if flags.UseDHT && !dhtAllowed {
		log.Println("[", ts.M.Info.Name, "] Can't use DHT because torrent is marked Private")
	}

	ts.Session = meta.SessionInfo{
		PeerID:        peerID(),
		Port:          listenPort,
		UseDHT:        dhtAllowed,
		FromMagnet:    fromMagnet,
		HaveTorrent:   false,
		ME:            &meta.MetaDataExchange{},
		OurExtensions: map[int]string{1: "ut_metadata"},
		OurAddresses:  map[string]bool{"127.0.0.1:" + strconv.Itoa(int(listenPort)): true},
	}
	ts.setHeader()

	if !ts.Session.FromMagnet {
		err = ts.load()
	}
	return ts, err
}

func (ts *TorrentSession) reload(metadata string) (err error) {
	var info file.InfoDict
	err = bencode.Unmarshal(bytes.NewReader([]byte(metadata)), &info)
	if err != nil {
		log.Println("[", ts.M.Info.Name, "] Error when reloading torrent: ", err)
		return
	}

	ts.M.Info = info
	err = ts.load()

	if ts.flags.Cacher != nil && ts.fileStore != nil {
		ts.fileStore = ts.flags.Cacher.NewCache(ts.M.InfoHash, ts.totalPieces, ts.M.Info.PieceLength, ts.totalSize, ts.fileStore)
	}
	return
}

func (ts *TorrentSession) load() error {
	log.Printf("[ %s ] Tracker: %v, Comment: %v, InfoHash: %x, Encoding: %v, Private: %v",
		ts.M.Info.Name, ts.M.AnnounceList, ts.M.Comment, ts.M.InfoHash, ts.M.Encoding, ts.M.Info.Private)
	if e := ts.M.Encoding; e != "" && e != "UTF-8" {
		err := fmt.Errorf("Unknown encoding %v", e)
		return err
	}

	dir := ts.flags.FileDir
	if len(ts.M.Info.Files) != 0 {
		torrentName := ts.M.Info.Name
		if torrentName == "" {
			torrentName = filepath.Base(ts.torrentFile)
		}
		// canonicalize the torrent path and make sure it doesn't start with ".."
		torrentName = path.Clean("/" + torrentName)
		dir += torrentName
		//Remove ".torrent" extension if present
		ex := defaults.Extension
		if strings.HasSuffix(strings.ToLower(dir), ex) {
			dir = dir[:len(dir)-len(ex)]
		}
	}

	fileSystem, err := ts.flags.FileSystemProvider.NewFS(dir)
	if err != nil {
		return err
	}

	ts.fileStore, ts.totalSize, err = file.NewFileStore(&ts.M.Info, fileSystem)
	if err != nil {
		return err
	}

	if ts.M.Info.PieceLength == 0 {
		err = fmt.Errorf("Bad PieceLength: %v", ts.M.Info.PieceLength)
		return err
	}

	ts.totalPieces = int(ts.totalSize / ts.M.Info.PieceLength)
	ts.lastPieceLength = int(ts.totalSize % ts.M.Info.PieceLength)
	if ts.lastPieceLength == 0 { // last piece is a full piece
		ts.lastPieceLength = int(ts.M.Info.PieceLength)
	} else {
		ts.totalPieces++
	}

	if ts.flags.MemoryPerTorrent < 0 {
		ts.maxActivePieces = 2147483640
		log.Printf("[ %s ] Max Active Pieces set to Unlimited\n", ts.M.Info.Name)
	} else {
		ts.maxActivePieces = int(int64(ts.flags.MemoryPerTorrent*1024*1024) / ts.M.Info.PieceLength)
		if ts.maxActivePieces == 0 {
			ts.maxActivePieces++
		}

		log.Printf("[ %s ] Max Active Pieces set to %v\n", ts.M.Info.Name, ts.maxActivePieces)
	}

	ts.goodPieces = 0
	if ts.flags.InitialCheck {
		start := time.Now()
		ts.goodPieces, _, ts.pieceSet, err = meta.CheckPieces(ts.fileStore, ts.totalSize, ts.M)
		end := time.Now()
		log.Printf("[ %s ] Computed missing pieces (%.2f seconds)\n", ts.M.Info.Name, end.Sub(start).Seconds())
		if err != nil {
			return err
		}
	} else if ts.flags.QuickResume {
		resumeFilePath := "./" + hex.EncodeToString([]byte(ts.M.InfoHash)) + "-haveBitset"
		if resumeFile, err := os.Open(resumeFilePath); err == nil {
			rfstat, _ := resumeFile.Stat()
			tBA := make([]byte, 2*rfstat.Size())
			count, _ := resumeFile.Read(tBA)
			ts.pieceSet = bitset.NewBitsetFromBytes(ts.totalPieces, tBA[:count])
			if ts.pieceSet == nil {
				return fmt.Errorf("[ %s ] Malformed resume data: %v", ts.M.Info.Name, resumeFilePath)
			}

			for i := 0; i < ts.totalPieces; i++ {
				if ts.pieceSet.IsSet(i) {
					ts.goodPieces++
				}
			}
			log.Printf("[ %s ] Got piece list from haveBitset file.\n", ts.M.Info.Name)
		} else {
			log.Printf("[ %s ] Couldn't open haveBitset file: %v", ts.M.Info.Name, err)
		}
	}

	if ts.pieceSet == nil { //Blank slate it is then.
		ts.pieceSet = bitset.NewBitset(ts.totalPieces)
		log.Printf("[ %s ] Starting from scratch.\n", ts.M.Info.Name)
	}

	bad := ts.totalPieces - ts.goodPieces
	left := uint64(bad) * uint64(ts.M.Info.PieceLength)
	if !ts.pieceSet.IsSet(ts.totalPieces - 1) {
		left = left - uint64(ts.M.Info.PieceLength) + uint64(ts.lastPieceLength)
	}
	ts.Session.Left = left

	log.Println("[", ts.M.Info.Name, "] Good pieces:", ts.goodPieces, "Bad pieces:", bad, "Bytes left:", left)

	// Enlarge any existing peers piece maps
	for _, p := range ts.peers {
		if p.Have.Len() != ts.totalPieces {
			if p.Have.Len() != 0 {
				log.Fatal("Expected p.have.n == 0")
			}
			p.Have = bitset.NewBitset(ts.totalPieces)
		}
	}
	ts.Session.HaveTorrent = true
	return nil
}

func (ts *TorrentSession) pieceLength(piece int) int {
	if piece < ts.totalPieces-1 {
		return int(ts.M.Info.PieceLength)
	}
	return ts.lastPieceLength
}

func (ts *TorrentSession) fetchTrackerInfo(event string) {
	m, si := ts.M, ts.Session
	log.Println("[", ts.M.Info.Name, "] Stats: Uploaded", si.Uploaded, "Downloaded", si.Downloaded, "Left", si.Left)
	ts.trackerReportChan <- trackerClient.ClientStatusReport{
		event, m.InfoHash, si.PeerID, si.Port, si.Uploaded, si.Downloaded, si.Left}
}

func (ts *TorrentSession) setHeader() {
	header := make([]byte, 68)
	copy(header, kBitTorrentHeader[0:])
	if ts.Session.UseDHT {
		header[27] = header[27] | 0x01
	}
	// Support Extension Protocol (BEP-0010)
	header[25] |= 0x10
	copy(header[28:48], []byte(ts.M.InfoHash))
	copy(header[48:68], []byte(ts.Session.PeerID))
	ts.torrentHeader = header
}

func (ts *TorrentSession) Header() (header []byte) {
	return ts.torrentHeader
}

// Try to connect if the peer is not already in our peers.
// Can be called from any goroutine.
func (ts *TorrentSession) HintNewPeer(peer string) {
	if len(ts.hintNewPeerChan) < cap(ts.hintNewPeerChan) { //We don't want to block the main loop because a single torrent is having problems
		select {
		case ts.hintNewPeerChan <- peer:
		case <-ts.ended:
		}
	} else {
		// log.Println("[", ts.M.Info.Name, "] New peer hint failed, because DoTorrent() hasn't been clearing out the channel.")
	}
}

func (ts *TorrentSession) tryNewPeer(peer string) bool {
	if (ts.Session.HaveTorrent || ts.Session.FromMagnet) && len(ts.peers) < conf.MAX_NUM_PEERS {
		if _, ok := ts.Session.OurAddresses[peer]; !ok {
			if _, ok := ts.peers[peer]; !ok {
				go ts.connectToPeer(peer)
				return true
			}
		} else {
			//	log.Println("[", ts.M.Info.Name, "] New peer hint rejected, because it's one of our addresses (", peer, ")")
		}
	}
	return false
}

func (ts *TorrentSession) connectToPeer(peer string) {
	conn, err := proxy.NetDial(ts.flags.Dial, "tcp", peer)
	if err != nil {
		// log.Println("[", ts.M.Info.Name, "] Failed to connect to", peer, err)
		return
	}

	_, err = conn.Write(ts.Header())
	if err != nil {
		log.Println("[", ts.M.Info.Name, "] Failed to send header to", peer, err)
		return
	}

	theirheader, err := listen.ReadHeader(conn)
	if err != nil {
		return
	}

	peersInfoHash := string(theirheader[8:28])
	id := string(theirheader[28:48])

	// log.Println("[", ts.M.Info.Name, "] Connected to", peer)
	ts.AddPeer(listen.NewBtConn(conn, nil, theirheader, peersInfoHash, id))
}

func (ts *TorrentSession) AcceptNewPeer(btconn *listen.BtConn) {
	_, err := btconn.Conn.Write(ts.Header())
	if err != nil {
		return
	}
	ts.AddPeer(btconn)
}

// Can be called from any goroutine
func (ts *TorrentSession) AddPeer(btconn *listen.BtConn) {
	if len(ts.addPeerChan) < cap(ts.addPeerChan) { //We don't want to block the main loop because a single torrent is having problems
		select {
		case ts.addPeerChan <- btconn:
		case <-ts.ended:
		}
	} else {
		// log.Println("[", ts.M.Info.Name, "] Add peer failed, because DoTorrent() hasn't been clearing out the channel.")
		btconn.Conn.Close()
	}
}

func (ts *TorrentSession) addPeerImp(btconn *listen.BtConn) {
	if !ts.Session.HaveTorrent && !ts.Session.FromMagnet {
		log.Println("[", ts.M.Info.Name, "] Rejecting peer because we don't have a torrent yet")
		btconn.Conn.Close()
		return
	}

	peerStr := btconn.Conn.RemoteAddr().String()

	if btconn.ID() == ts.Session.PeerID {
		log.Println("[", ts.M.Info.Name, "] Rejecting self-connection:", peerStr, "<->", btconn.Conn.LocalAddr())
		ts.Session.OurAddresses[btconn.Conn.LocalAddr().String()] = true
		ts.Session.OurAddresses[peerStr] = true
		btconn.Conn.Close()
		return
	}

	for _, p := range ts.peers {
		if p.ID == btconn.ID() {
			log.Println("[", ts.M.Info.Name, "] Rejecting peer because already have a peer with the same id")
			btconn.Conn.Close()
			return
		}
	}

	// log.Println("[", ts.M.Info.Name, "] Adding peer", peer)
	if len(ts.peers) >= conf.MAX_NUM_PEERS {
		log.Println("[", ts.M.Info.Name, "] We have enough peers. Rejecting additional peer", peerStr)
		btconn.Conn.Close()
		return
	}

	theirheader := btconn.ReadHeader()
	if ts.Session.UseDHT {
		// If 128, then it supports DHT.
		if int(theirheader[7])&0x01 == 0x01 {
			// It's OK if we know this node already. The DHT engine will
			// ignore it accordingly.
			go ts.dht.AddNode(peerStr)
		}
	}

	ps := peer.NewPeerState(btconn.Conn)
	ps.Address = peerStr
	ps.ID = btconn.ID()

	// By default, a peer has no pieces. If it has pieces, it should send
	// a BITFIELD message as a first message
	// If the torrent has not been received yet, ts.totalPieces will be 0, and
	// the "have" map will have to be enlarged later when ts.totalPieces is
	// learned.

	ps.Have = bitset.NewBitset(ts.totalPieces)

	ts.peers[peerStr] = ps
	go ps.PeerWriter(ts.peerMessageChan)
	go ps.PeerReader(ts.peerMessageChan)

	if int(theirheader[5])&0x10 == 0x10 {
		ps.SendExtensions(ts.Session.Port)
	} else if ts.pieceSet != nil {
		ps.SendBitfield(ts.pieceSet)
	}
}

func (ts *TorrentSession) ClosePeer(peer *peer.PeerState) {
	if ts.Session.ME != nil && !ts.Session.ME.Transferring {
		ts.Session.ME.Transferring = false
	}

	//log.Println("[", ts.M.Info.Name, "] Closing peer", peer.address)
	_ = ts.removeRequests(peer)
	peer.Close()
	delete(ts.peers, peer.Address)
}

func (ts *TorrentSession) deadlockDetector() {
	// Wait for a heartbeat before we start deadlock detection.
	// This handle the case where it takes a long time to find
	// a tracker.
	<-ts.heartbeat
	lastHeartbeat := time.Now()
	for {
		select {
		case <-ts.heartbeat:
			lastHeartbeat = time.Now()
		case <-time.After(15 * time.Second):
			age := time.Now().Sub(lastHeartbeat)
			log.Println("[", ts.M.Info.Name, "] Starvation or deadlock of main thread detected. Look in the stack dump for what DoTorrent() is currently doing")
			log.Println("[", ts.M.Info.Name, "] Last heartbeat", age.Seconds(), "seconds ago")
			panic("[" + ts.M.Info.Name + "] Killed by deadlock detector")
		}
	}
}

func (ts *TorrentSession) Quit() (err error) {
	select {
	case ts.quit <- true:
	case <-ts.ended:
	}
	return
}

func (ts *TorrentSession) Shutdown() (err error) {
	close(ts.ended)

	if ts.fileStore != nil {
		err = ts.fileStore.Close()
		if err != nil {
			log.Println("[", ts.M.Info.Name, "] Error closing filestore:", err)
		}
	}

	for _, peer := range ts.peers {
		peer.Close()
	}

	return
}

func (ts *TorrentSession) DoTorrent() {
	ts.heartbeat = make(chan bool, 1)
	if ts.flags.UseDeadlockDetector {
		go ts.deadlockDetector()
	}

	if ts.flags.Cacher != nil && ts.fileStore != nil {
		ts.fileStore = ts.flags.Cacher.NewCache(ts.M.InfoHash, ts.totalPieces, ts.M.Info.PieceLength, ts.totalSize, ts.fileStore)
	}

	heartbeatDuration := 1 * time.Second
	heartbeatChan := time.Tick(heartbeatDuration)

	keepAliveChan := time.Tick(60 * time.Second)
	var retrackerChan <-chan time.Time
	ts.hintNewPeerChan = make(chan string, conf.MAX_NUM_PEERS)
	ts.addPeerChan = make(chan *listen.BtConn, conf.MAX_NUM_PEERS)
	if !ts.trackerLessMode {
		// Start out polling tracker every 20 seconds until we get a response.
		// Maybe be exponential backoff here?
		retrackerChan = time.Tick(20 * time.Second)
		ts.trackerInfoChan = make(chan *meta.TrackerResponse)
		ts.trackerReportChan = make(chan trackerClient.ClientStatusReport)
		trackerClient.StartTrackerClient(ts.flags.Dial, ts.M.Announce, ts.M.AnnounceList, ts.trackerInfoChan, ts.trackerReportChan)
	}

	if ts.Session.UseDHT {
		ts.dht.PeersRequest(ts.M.InfoHash, true)
	}

	if !ts.trackerLessMode && ts.Session.HaveTorrent {
		ts.fetchTrackerInfo("started")
	}

	defer ts.Shutdown()
	go func() {
		for {
			for _, p := range ts.peers {
				str := "PEER FILE UPDATE TEST"
				p.SendMessage([]byte(str))
			}
			time.Sleep(5 * time.Second)
		}
	}()
	lastDownloaded := ts.Session.Downloaded
	for {
		if !ts.execOnSeedingDone && ts.goodPieces == ts.totalPieces {
			ts.execOnSeeding()
			ts.execOnSeedingDone = true
		}
		select {
		case <-ts.chokePolicyHeartbeat:
			ts.chokePeers()
		case hintNewPeer := <-ts.hintNewPeerChan:
			ts.tryNewPeer(hintNewPeer)
		case btconn := <-ts.addPeerChan:
			ts.addPeerImp(btconn)
		case <-retrackerChan:
			if !ts.trackerLessMode {
				ts.fetchTrackerInfo("")
			}
		case ti := <-ts.trackerInfoChan:
			ts.ti = ti
			log.Println("[", ts.M.Info.Name, "] Torrent has", ts.ti.Complete, "seeders and", ts.ti.Incomplete, "leachers")
			if !ts.trackerLessMode {
				newPeerCount := 0
				{
					peers := ts.ti.Peers
					if len(peers) > 0 {
						const peerLen = 6
						log.Println("[", ts.M.Info.Name, "] Tracker gave us", len(peers)/peerLen, "peers")
						for i := 0; i < len(peers); i += peerLen {
							peer := nettools.BinaryToDottedPort(peers[i : i+peerLen])
							if ts.tryNewPeer(peer) {
								newPeerCount++
							}
						}
					}
				}
				{
					peers6 := ts.ti.Peers6
					if len(peers6) > 0 {
						const peerLen = 18
						log.Println("[", ts.M.Info.Name, "] Tracker gave us", len(peers6)/peerLen, "IPv6 peers")
						for i := 0; i < len(peers6); i += peerLen {
							peerEntry := peers6[i : i+peerLen]
							host := net.IP(peerEntry[0:16])
							port := int((uint(peerEntry[16]) << 8) | uint(peerEntry[17]))
							peer := net.JoinHostPort(host.String(), strconv.Itoa(port))
							if ts.tryNewPeer(peer) {
								newPeerCount++
							}
						}
					}
				}
				log.Println("[", ts.M.Info.Name, "] Contacting", newPeerCount, "new peers")
			}

			interval := ts.ti.Interval
			minInterval := uint(120)
			maxInterval := uint(24 * 3600)
			if interval < minInterval {
				interval = minInterval
			} else if interval > maxInterval {
				interval = maxInterval
			}
			log.Println("[", ts.M.Info.Name, "] ..checking again in", interval, "seconds")
			retrackerChan = time.Tick(time.Duration(interval) * time.Second)

		case pm := <-ts.peerMessageChan:
			peer, message := pm.Peer, pm.Message
			peer.LastReadTime = time.Now()
			err2 := ts.DoMessage(peer, message)
			if err2 != nil {
				if err2 != io.EOF {
					log.Println("[", ts.M.Info.Name, "] Closing peer", peer.Address, "because", err2)
				}
				ts.ClosePeer(peer)
			}
		case <-heartbeatChan:
			if ts.flags.UseDeadlockDetector {
				ts.heartbeat <- true
			}
			ratio := float64(0.0)
			if ts.Session.Downloaded > 0 {
				ratio = float64(ts.Session.Uploaded) / float64(ts.Session.Downloaded)
			}
			speed := humanSize(float64(ts.Session.Downloaded-lastDownloaded) / heartbeatDuration.Seconds())
			lastDownloaded = ts.Session.Downloaded
			log.Printf("[ %s ] Peers: %d downloaded: %d (%s/s) uploaded: %d ratio: %f pieces: %d/%d\n",
				ts.M.Info.Name,
				len(ts.peers),
				ts.Session.Downloaded,
				speed,
				ts.Session.Uploaded,
				ratio,
				ts.goodPieces,
				ts.totalPieces)
			if ts.totalPieces != 0 && ts.goodPieces == ts.totalPieces && ratio >= ts.flags.SeedRatio {
				log.Println("[", ts.M.Info.Name, "] Achieved target seed ratio", ts.flags.SeedRatio)
				return
			}
			if len(ts.peers) < conf.TARGET_NUM_PEERS && (ts.totalPieces == 0 || ts.goodPieces < ts.totalPieces) {
				if ts.Session.UseDHT {
					go ts.dht.PeersRequest(ts.M.InfoHash, true)
				}
				if !ts.trackerLessMode {
					if ts.ti == nil || ts.ti.Complete > 100 {
						ts.fetchTrackerInfo("")
					}
				}
			}
		case <-keepAliveChan:
			now := time.Now()
			for _, peer := range ts.peers {
				if peer.LastReadTime.Second() != 0 && now.Sub(peer.LastReadTime) > 3*time.Minute {
					// log.Println("[", ts.M.Info.Name, "] Closing peer", peer.address, "because timed out")
					ts.ClosePeer(peer)
					continue
				}
				err2 := ts.doCheckRequests(peer)
				if err2 != nil {
					if err2 != io.EOF {
						log.Println("[", ts.M.Info.Name, "] Closing peer", peer.Address, "because", err2)
					}
					ts.ClosePeer(peer)
					continue
				}
				peer.KeepAlive(now)
			}

		case <-ts.quit:
			log.Println("[", ts.M.Info.Name, "] Quitting torrent session")
			ts.fetchTrackerInfo("stopped")
			time.Sleep(10 * time.Millisecond)
			return
		}
	}
}

func (ts *TorrentSession) chokePeers() (err error) {
	// log.Printf("[ %s ] Choking peers", ts.M.Info.Name)
	peers := ts.peers
	chokers := make([]choker.Choker, 0, len(peers))
	for _, peer := range peers {
		if peer.PeerInterested {
			peer.ComputeDownloadRate()
			// log.Printf("%s %g bps", peer.address, peer.DownloadBPS())
			chokers = append(chokers, choker.Choker(peer))
		}
	}
	var unchokeCount int
	unchokeCount, err = ts.chokePolicy.Choke(chokers)
	if err != nil {
		return
	}
	for i, c := range chokers {
		shouldChoke := i >= unchokeCount
		if peer, ok := c.(*peer.PeerState); ok {
			if shouldChoke != peer.AmChoking {
				//	log.Printf("[ %s ] Changing choke status %v -> %v", ts.M.Info.Name, peer.address, shouldChoke)
				peer.SetChoke(shouldChoke)
			}
		}
	}
	return
}

func MessagePeer(peer *peer.PeerState) {
	str := "PEER FILE UPDATE PING"
	peer.SendMessage([]byte(str))
}

func (ts *TorrentSession) RequestBlock(p *peer.PeerState) error {
	if !ts.Session.HaveTorrent { // We can't request a block without a torrent
		return nil
	}

	for k := range ts.activePieces {
		if p.Have.IsSet(k) {
			err := ts.RequestBlock2(p, k, false)
			if err != io.EOF {
				return err
			}
		}
	}

	if len(ts.activePieces) >= ts.maxActivePieces {
		return nil
	}

	// No active pieces. (Or no suitable active pieces.) Pick one
	piece := ts.ChoosePiece(p)
	if piece < 0 {
		// No unclaimed pieces. See if we can double-up on an active piece
		for k := range ts.activePieces {
			if p.Have.IsSet(k) {
				err := ts.RequestBlock2(p, k, true)
				if err != io.EOF {
					return err
				}
			}
		}
	}

	if piece < 0 {
		p.SetInterested(false)
		return nil
	}
	pieceLength := ts.pieceLength(piece)
	pieceCount := (pieceLength + peer.STANDARD_BLOCK_LENGTH - 1) / peer.STANDARD_BLOCK_LENGTH
	ts.activePieces[piece] = &ActivePiece{make([]int, pieceCount), make([]byte, pieceLength)}
	return ts.RequestBlock2(p, piece, false)
}

func (ts *TorrentSession) ChoosePiece(p *peer.PeerState) (piece int) {
	n := ts.totalPieces
	start := rand.Intn(n)
	piece = ts.checkRange(p, start, n)
	if piece == -1 {
		piece = ts.checkRange(p, 0, start)
	}
	return
}

// checkRange returns the first piece in range start..end that is not in the
// torrent's pieceSet but is in the peer's pieceSet.
func (ts *TorrentSession) checkRange(p *peer.PeerState, start, end int) (piece int) {
	clampedEnd := min(end, min(p.Have.Len(), ts.pieceSet.Len()))
	for i := start; i < clampedEnd; i++ {
		if (!ts.pieceSet.IsSet(i)) && p.Have.IsSet(i) {
			if _, ok := ts.activePieces[i]; !ok {
				return i
			}
		}
	}
	return -1
}

func (ts *TorrentSession) RequestBlock2(p *peer.PeerState, piece int, endGame bool) (err error) {
	v := ts.activePieces[piece]
	block := v.chooseBlockToDownload(endGame)
	if block >= 0 {
		ts.requestBlockImp(p, piece, block, true)
	} else {
		return io.EOF
	}
	return
}

// Request or cancel a block
func (ts *TorrentSession) requestBlockImp(p *peer.PeerState, piece int, block int, request bool) {
	begin := block * peer.STANDARD_BLOCK_LENGTH
	req := make([]byte, 13)
	opcode := byte(conf.REQUEST)
	if !request {
		opcode = byte(conf.CANCEL)
	}
	length := peer.STANDARD_BLOCK_LENGTH
	if piece == ts.totalPieces-1 {
		left := ts.lastPieceLength - begin
		if left < length {
			length = left
		}
	}
	// log.Println("[", ts.M.Info.Name, "] Requesting block", piece, ".", block, length, request)
	req[0] = opcode
	convert.Uint32ToBytes(req[1:5], uint32(piece))
	convert.Uint32ToBytes(req[5:9], uint32(begin))
	convert.Uint32ToBytes(req[9:13], uint32(length))
	requestIndex := (uint64(piece) << 32) | uint64(begin)
	if !request {
		delete(p.OurRequests, requestIndex)
	} else {
		p.OurRequests[requestIndex] = time.Now()
	}
	p.SendMessage(req)
	return
}

func (ts *TorrentSession) RecordBlock(p *peer.PeerState, piece, begin, length uint32) (err error) {
	block := begin / peer.STANDARD_BLOCK_LENGTH
	// log.Println("[", ts.M.Info.Name, "] Received block", piece, ".", block)
	requestIndex := (uint64(piece) << 32) | uint64(begin)
	delete(p.OurRequests, requestIndex)
	v, ok := ts.activePieces[int(piece)]
	if ok {
		requestCount := v.recordBlock(int(block))
		if requestCount > 1 {
			// Someone else has also requested this, so send cancel notices
			for _, peer := range ts.peers {
				if p != peer {
					if _, ok := peer.OurRequests[requestIndex]; ok {
						ts.requestBlockImp(peer, int(piece), int(block), false)
						requestCount--
					}
				}
			}
		}
		ts.Session.Downloaded += uint64(length)
		if v.isComplete() {
			delete(ts.activePieces, int(piece))

			ok, err = meta.CheckPiece(v.buffer, ts.M, int(piece))
			if !ok || err != nil {
				log.Println("[", ts.M.Info.Name, "] Closing peer that sent a bad piece", piece, p.ID, err)
				p.Close()
				return
			}
			ts.fileStore.WritePiece(v.buffer, int(piece))
			ts.Session.Left -= uint64(len(v.buffer))
			ts.pieceSet.Set(int(piece))
			ts.goodPieces++
			if ts.flags.QuickResume {
				ioutil.WriteFile("./"+hex.EncodeToString([]byte(ts.M.InfoHash))+"-haveBitset", ts.pieceSet.Bytes(), 0777)
			}
			var percentComplete float32
			if ts.totalPieces > 0 {
				percentComplete = float32(ts.goodPieces*100) / float32(ts.totalPieces)
			}
			log.Println("[", ts.M.Info.Name, "] Have", ts.goodPieces, "of", ts.totalPieces,
				"pieces", percentComplete, "% complete")
			if ts.goodPieces == ts.totalPieces {
				if !ts.trackerLessMode {
					ts.fetchTrackerInfo("completed")
				}
				// TODO: Drop connections to all seeders.
			}
			for _, p := range ts.peers {
				if p.Have != nil {
					if int(piece) < p.Have.Len() && p.Have.IsSet(int(piece)) {
						// We don't do anything special. We rely on the caller
						// to decide if this peer is still interesting.
					} else {
						// log.Println("[", ts.M.Info.Name, "] ...telling ", p)
						haveMsg := make([]byte, 5)
						haveMsg[0] = conf.HAVE
						convert.Uint32ToBytes(haveMsg[1:5], piece)
						p.SendMessage(haveMsg)
					}
				}
			}
		}
	} else {
		log.Println("[", ts.M.Info.Name, "] Received a block we already have.", piece, block, p.Address)
	}
	return
}

func (ts *TorrentSession) doChoke(p *peer.PeerState) (err error) {
	p.PeerChoking = true
	err = ts.removeRequests(p)
	return
}

func (ts *TorrentSession) removeRequests(p *peer.PeerState) (err error) {
	for k := range p.OurRequests {
		piece := int(k >> 32)
		begin := int(k & 0xffffffff)
		block := begin / peer.STANDARD_BLOCK_LENGTH
		// log.Println("[", ts.M.Info.Name, "] Forgetting we requested block ", piece, ".", block)
		ts.removeRequest(piece, block)
	}
	p.OurRequests = make(map[uint64]time.Time, peer.MAX_OUR_REQUESTS)
	return
}

func (ts *TorrentSession) removeRequest(piece, block int) {
	v, ok := ts.activePieces[piece]
	if ok && v.downloaderCount[block] > 0 {
		v.downloaderCount[block]--
	}
}

func (ts *TorrentSession) doCheckRequests(p *peer.PeerState) (err error) {
	now := time.Now()
	for k, v := range p.OurRequests {
		if now.Sub(v).Seconds() > 30 {
			piece := int(k >> 32)
			block := int(k&0xffffffff) / peer.STANDARD_BLOCK_LENGTH
			// log.Println("[", ts.M.Info.Name, "] timing out request of", piece, ".", block)
			ts.removeRequest(piece, block)
		}
	}
	return
}

func (ts *TorrentSession) DoMessage(p *peer.PeerState, message []byte) (err error) {
	if message == nil {
		return io.EOF // The reader or writer goroutine has exited
	}
	if len(message) == 0 { // keep alive
		return
	}

	if ts.Session.HaveTorrent {
		err = ts.generalMessage(message, p)
	} else {
		err = ts.extensionMessage(message, p)
	}
	return
}

func (ts *TorrentSession) extensionMessage(message []byte, p *peer.PeerState) (err error) {
	if message[0] == conf.EXTENSION {
		err := ts.DoExtension(message[1:], p)
		if err != nil {
			log.Printf("[ %s ] Failed extensions for %s: %s\n", ts.M.Info.Name, p.Address, err)
		}
	}
	return
}

func (ts *TorrentSession) generalMessage(message []byte, p *peer.PeerState) (err error) {
	messageID := message[0]

	switch messageID {
	case conf.CHOKE:
		// log.Println("[", ts.M.Info.Name, "] choke", p.address)
		if len(message) != 1 {
			return errors.New("Unexpected length")
		}
		err = ts.doChoke(p)
	case conf.UNCHOKE:
		// log.Println("[", ts.M.Info.Name, "] unchoke", p.address)
		if len(message) != 1 {
			return errors.New("Unexpected length")
		}
		p.PeerChoking = false
		for i := 0; i < peer.MAX_OUR_REQUESTS; i++ {
			err = ts.RequestBlock(p)
			if err != nil {
				return
			}
		}
	case conf.INTERESTED:
		// log.Println("[", ts.M.Info.Name, "] interested", p)
		if len(message) != 1 {
			return errors.New("Unexpected length")
		}
		p.PeerInterested = true
		ts.chokePeers()
	case conf.NOT_INTERESTED:
		// log.Println("[", ts.M.Info.Name, "] not interested", p)
		if len(message) != 1 {
			return errors.New("Unexpected length")
		}
		p.PeerInterested = false
		ts.chokePeers()
	case conf.HAVE:
		if len(message) != 5 {
			return errors.New("Unexpected length")
		}
		n := convert.BytesToUint32(message[1:])
		if n < uint32(p.Have.Len()) {
			p.Have.Set(int(n))
			if !p.AmInterested && !ts.pieceSet.IsSet(int(n)) {
				p.SetInterested(true)
			}
		} else {
			return errors.New("have index is out of range")
		}
	case conf.BITFIELD:
		// log.Println("[", ts.M.Info.Name, "] bitfield", p.address)
		if !p.CanReceive {
			return errors.New("Late bitfield operation")
		}
		p.Have = bitset.NewBitsetFromBytes(ts.totalPieces, message[1:])
		if p.Have == nil {
			return errors.New("Invalid bitfield data")
		}
		ts.checkInteresting(p)
	case conf.REQUEST:
		// log.Println("[", ts.M.Info.Name, "] request", p.address)
		if len(message) != 13 {
			return errors.New("Unexpected message length")
		}
		index := convert.BytesToUint32(message[1:5])
		begin := convert.BytesToUint32(message[5:9])
		length := convert.BytesToUint32(message[9:13])
		if index >= uint32(p.Have.Len()) {
			return errors.New("piece out of range")
		}
		if !ts.pieceSet.IsSet(int(index)) {
			return errors.New("we don't have that piece")
		}
		if int64(begin) >= ts.M.Info.PieceLength {
			return errors.New("begin out of range")
		}
		if int64(begin)+int64(length) > ts.M.Info.PieceLength {
			return errors.New("begin + length out of range")
		}
		// TODO: Asynchronous
		// p.AddRequest(index, begin, length)
		return ts.sendRequest(p, index, begin, length)
	case conf.PIECE:
		// piece
		if len(message) < 9 {
			return errors.New("unexpected message length")
		}
		index := convert.BytesToUint32(message[1:5])
		begin := convert.BytesToUint32(message[5:9])
		length := len(message) - 9
		if index >= uint32(p.Have.Len()) {
			return errors.New("piece out of range")
		}
		if ts.pieceSet.IsSet(int(index)) {
			// We already have that piece, keep going
			break
		}
		if int64(begin) >= ts.M.Info.PieceLength {
			return errors.New("begin out of range")
		}
		if int64(begin)+int64(length) > ts.M.Info.PieceLength {
			return errors.New("begin + length out of range")
		}
		if length > 128*1024 {
			return errors.New("Block length too large")
		}
		v, ok := ts.activePieces[int(index)]
		if !ok {
			return errors.New("Received piece data we weren't expecting")
		}
		copy(v.buffer[begin:], message[9:])

		p.CreditDownload(int64(length))
		ts.RecordBlock(p, index, begin, uint32(length))
		err = ts.RequestBlock(p)
	case conf.CANCEL:
		// log.Println("[", ts.M.Info.Name, "] cancel")
		if len(message) != 13 {
			return errors.New("Unexpected message length")
		}
		index := convert.BytesToUint32(message[1:5])
		begin := convert.BytesToUint32(message[5:9])
		length := convert.BytesToUint32(message[9:13])
		if index >= uint32(p.Have.Len()) {
			return errors.New("piece out of range")
		}
		if !ts.pieceSet.IsSet(int(index)) {
			return errors.New("we don't have that piece")
		}
		if int64(begin) >= ts.M.Info.PieceLength {
			return errors.New("begin out of range")
		}
		if int64(begin)+int64(length) > ts.M.Info.PieceLength {
			return errors.New("begin + length out of range")
		}
		if length != peer.STANDARD_BLOCK_LENGTH {
			return errors.New("Unexpected block length")
		}
		p.CancelRequest(index, begin, length)
	case conf.PORT:
		// TODO: Implement this message.
		// We see peers sending us 16K byte messages here, so
		// it seems that we don't understand what this is.
		if len(message) != 3 {
			return fmt.Errorf("Unexpected length for port message: %d", len(message))
		}
		go ts.dht.AddNode(p.Address)
	case conf.EXTENSION:
		err := ts.DoExtension(message[1:], p)
		if err != nil {
			log.Printf("[ %s ] Failed extensions for %s: %s\n", ts.M.Info.Name, p.Address, err)
		}

		if ts.Session.HaveTorrent {
			p.SendBitfield(ts.pieceSet)
		}
	default:
		return fmt.Errorf("unknown message id: %d", messageID)
	}

	if messageID != conf.EXTENSION {
		p.CanReceive = false
	}

	return
}

type ExtensionHandshake struct {
	M      map[string]int `bencode:"m"`
	P      uint16         `bencode:"p"`
	V      string         `bencode:"v"`
	Yourip string         `bencode:"yourip"`
	Ipv6   string         `bencode:"ipv6"`
	Ipv4   string         `bencode:"ipv4"`
	Reqq   uint16         `bencode:"reqq"`

	MetadataSize uint `bencode:"metadata_size"`
}

func (ts *TorrentSession) DoExtension(msg []byte, p *peer.PeerState) (err error) {
	var h ExtensionHandshake
	if msg[0] == conf.EXTENSION_HANDSHAKE {
		err = bencode.Unmarshal(bytes.NewReader(msg[1:]), &h)
		if err != nil {
			log.Println("[", ts.M.Info.Name, "] Error when unmarshaling extension handshake")
			return err
		}

		p.TheirExtensions = make(map[string]int)
		for name, code := range h.M {
			p.TheirExtensions[name] = code
		}

		if ts.Session.HaveTorrent || ts.Session.ME != nil && ts.Session.ME.Transferring {
			return
		}

		// Fill metadata info
		if h.MetadataSize != uint(0) {
			nPieces := uint(math.Ceil(float64(h.MetadataSize) / float64(16*1024)))
			ts.Session.ME.Pieces = make([][]byte, nPieces)
		}

		if _, ok := p.TheirExtensions["ut_metadata"]; ok {
			ts.Session.ME.Transferring = true
			p.SendMetadataRequest(0)
		}

	} else if ext, ok := ts.Session.OurExtensions[int(msg[0])]; ok {
		switch ext {
		case "ut_metadata":
			ts.DoMetadata(msg[1:], p)
		default:
			log.Println("[", ts.M.Info.Name, "] Unknown extension: ", ext)
		}
	} else {
		log.Println("[", ts.M.Info.Name, "] Unknown extension: ", int(msg[0]))
	}
	return nil
}

type MetadataMessage struct {
	MsgType   uint8 `bencode:"msg_type"`
	Piece     uint  `bencode:"piece"`
	TotalSize uint  `bencode:"total_size"`
}

//From bittorrent.org, a bep 9 data message is structured as follows:
//d8:msg_typei1e5:piecei0e10:total_sizei34256eexxxx
//xxxx being the piece data
//So, simplest approach: search for 'ee' as the end of bencoded data
func getMetadataPiece(msg []byte) ([]byte, error) {
	for i := 0; i < len(msg)-1; i++ {
		if msg[i] == 'e' && msg[i+1] == 'e' {
			return msg[i+2:], nil
		}
	}
	return nil, errors.New("Couldn't find an appropriate end to the bencoded message")
}

func (ts *TorrentSession) DoMetadata(msg []byte, p *peer.PeerState) {
	var message MetadataMessage
	err := bencode.Unmarshal(bytes.NewReader(msg), &message)
	if err != nil {
		log.Println("[", ts.M.Info.Name, "] Error when parsing metadata:", err)
		return
	}

	mt := message.MsgType
	switch mt {
	case conf.METADATA_REQUEST:
		//TODO: Answer to metadata request
	case conf.METADATA_DATA:
		if ts.Session.HaveTorrent {
			log.Println("[", ts.M.Info.Name, "] Received metadata we don't need, from", p.Address)
			return
		}

		piece, err := getMetadataPiece(msg)
		if err != nil {
			log.Println("[", ts.M.Info.Name, "] Error when getting metadata piece: ", err)
			return
		}
		ts.Session.ME.Pieces[message.Piece] = piece

		finished := true
		for idx, data := range ts.Session.ME.Pieces {
			if len(data) == 0 {
				p.SendMetadataRequest(idx)
				finished = false
			}
		}

		if !finished {
			break
		}

		log.Println("[", ts.M.Info.Name, "] Finished downloading metadata!")
		var full bytes.Buffer
		for _, piece := range ts.Session.ME.Pieces {
			full.Write(piece)
		}
		b := full.Bytes()

		// Verify sha
		sha := sha1.New()
		sha.Write(b)
		actual := string(sha.Sum(nil))
		if actual != ts.M.InfoHash {
			log.Printf("[ %s ] Invalid metadata; got %x\n", ts.M.Info.Name, actual)
		}

		metadata := string(b)
		err = meta.SaveMetaInfo(metadata)
		if err != nil {
			return
		}
		ts.reload(metadata)
	case conf.METADATA_REJECT:
		log.Printf("[ %s ] %s didn't want to send piece %d\n", ts.M.Info.Name, p.Address, message.Piece)
	default:
		log.Println("[", ts.M.Info.Name, "] Didn't understand metadata extension type: ", mt)
	}
}

func (ts *TorrentSession) sendRequest(peer *peer.PeerState, index, begin, length uint32) (err error) {
	if !peer.AmChoking {
		// log.Println("[", ts.M.Info.Name, "] Sending block", index, begin, length)
		buf := make([]byte, length+9)
		buf[0] = conf.PIECE
		convert.Uint32ToBytes(buf[1:5], index)
		convert.Uint32ToBytes(buf[5:9], begin)
		_, err = ts.fileStore.ReadAt(buf[9:],
			int64(index)*ts.M.Info.PieceLength+int64(begin))
		if err != nil {
			return
		}
		peer.SendMessage(buf)
		ts.Session.Uploaded += uint64(length)
	}
	return
}

func (ts *TorrentSession) checkInteresting(p *peer.PeerState) {
	p.SetInterested(ts.isInteresting(p))
}

func (ts *TorrentSession) isInteresting(p *peer.PeerState) bool {
	for i := 0; i < ts.totalPieces; i++ {
		if !ts.pieceSet.IsSet(i) && p.Have.IsSet(i) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func humanSize(value float64) string {
	switch {
	case value > 1<<30:
		return fmt.Sprintf("%.2f GB", value/(1<<30))
	case value > 1<<20:
		return fmt.Sprintf("%.2f MB", value/(1<<20))
	case value > 1<<10:
		return fmt.Sprintf("%.2f kB", value/(1<<10))
	}
	return fmt.Sprintf("%.2f B", value)
}

func (ts *TorrentSession) execOnSeeding() {
	cmd := exec.Command(ts.flags.ExecOnSeeding)
	cmd.Env = []string{
		fmt.Sprintf("TORRENT_FILE=%s", ts.torrentFile),
		fmt.Sprintf("TORRENT_INFOHASH=%x", ts.M.InfoHash),
	}
	starterr := cmd.Start()
	if starterr != nil {
		log.Printf("[ %s ] Error starting '%s': %v\n", ts.M.Info.Name, ts.flags.ExecOnSeeding, starterr)
		return
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("[ %s ] Error while executing '%s': %v\n", ts.M.Info.Name, ts.flags.ExecOnSeeding, err)
		} else {
			log.Printf("[ %s ] Executing finished on '%s'\n", ts.M.Info.Name, ts.flags.ExecOnSeeding)
		}
	}()
}
