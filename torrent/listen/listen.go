package listen

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/jackpal/gateway"
	"github.com/rasaford/bitsync/torrent/conf"
	"github.com/rasaford/bitsync/torrent/nat"
)

// btConn wraps an incoming network connection and contains metadata that helps
// identify which active torrentSession it's relevant for.
type BtConn struct {
	Conn       net.Conn
	RemoteAddr net.Addr
	header     []byte
	Infohash   string
	id         string
}

func NewBtConn(conn net.Conn, remoteAddr net.Addr, header []byte, infoHash string, id string) *BtConn {
	return &BtConn{
		Conn:       conn,
		RemoteAddr: remoteAddr,
		header:     header,
		Infohash:   infoHash,
		id:         id,
	}
}

// ID returns the id of the BtConn
func (b *BtConn) ID() string {
	return b.id
}

func (b *BtConn) ReadHeader() []byte {
	return b.header
}

// ListenForPeerConnections listens on a TCP port for incoming connections and
// demuxes them to the appropriate active torrentSession based on the InfoHash
// in the header.
func ListenForPeerConnections(flags *conf.Flags) (conChan chan *BtConn, listenPort int, err error) {
	listener, listenPort, err := CreateListener(flags)
	if err != nil {
		return
	}
	conChan = make(chan *BtConn)
	_, portstring, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		log.Printf("Listener failed while finding the host/port for %v: %v", portstring, err)
		return
	}
	go func() {
		for {
			var conn net.Conn
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Listener accept failed:", err)
				continue
			}
			header, err := ReadHeader(conn)
			log.Println(string(header))
			if err != nil {
				log.Println("Error reading header: ", err)
				continue
			}
			peersInfoHash := string(header[8:28])
			id := string(header[28:48])
			conChan <- NewBtConn(conn, conn.RemoteAddr(), header, peersInfoHash, id)
		}
	}()
	return
}

func CreateListener(flags *conf.Flags) (listener net.Listener, externalPort int, err error) {
	nat, err := CreatePortMapping(flags)
	if err != nil {
		err = fmt.Errorf("Unable to create NAT: %v", err)
		return
	}
	listenPort := flags.Port
	if nat != nil {
		var external net.IP
		if external, err = nat.GetExternalAddress(); err != nil {
			err = fmt.Errorf("Unable to get external IP address from NAT: %v", err)
			return
		}
		log.Println("External ip address: ", external)
		if listenPort, err = chooseListenPort(nat, listenPort); err != nil {
			log.Println("Could not choose listen port.", err)
			log.Println("Peer connectivity will be affected.")
		}
	}
	listener, err = net.ListenTCP("tcp", &net.TCPAddr{Port: listenPort})
	if err != nil {
		log.Fatal("Listen failed:", err)
	}
	if listenPort == 0 {
		_, portString, _ := net.SplitHostPort(listener.Addr().String())
		listenPort, _ = strconv.Atoi(portString)
		flags.Port = listenPort
	}
	log.Println("Listening for peers on port:", listenPort)
	externalPort = listenPort
	return
}

// createPortMapping creates a NAT port mapping, or nil if none requested or found.
func CreatePortMapping(flags *conf.Flags) (translate nat.NAT, err error) {
	if flags.UseUPnP && flags.UseNATPMP {
		err = fmt.Errorf("Cannot specify both -useUPnP and -useNATPMP")
		return
	}
	if flags.UseUPnP {
		log.Println("Using UPnP to open port.")
		// nat, err = Discover()
	}
	if flags.UseNATPMP {
		var gatewayIP net.IP
		if flags.Gateway == "" {
			log.Printf("useNATPMP but gateway not provided, trying discovery")
			gatewayIP, err = gateway.DiscoverGateway()
			if err != nil {
				return
			}
			log.Printf("...discovered gateway IP: %s", gatewayIP)
		} else {
			gatewayIP = net.ParseIP(flags.Gateway)
		}
		log.Println("Using NAT-PMP to open port.")
		if gatewayIP == nil {
			err = fmt.Errorf("Could not parse gateway %q", flags.Gateway)
		}
		translate = nat.NewNatPMP(gatewayIP)
	}
	return
}

func chooseListenPort(nat nat.NAT, externalPort int) (listenPort int, err error) {
	// TODO: Unmap port when exiting. (Right now we never exit cleanly.)
	// TODO: Defend the port, remap when router reboots
	listenPort, err = nat.AddPortMapping("tcp", externalPort, externalPort,
		"Taipei-Torrent port "+strconv.Itoa(externalPort), 360000)
	if err != nil {
		return
	}
	return
}

func ReadHeader(conn net.Conn) (h []byte, err error) {
	header := make([]byte, 68)
	_, err = conn.Read(header[0:1])
	if err != nil {
		err = fmt.Errorf("Couldn't read 1st byte: %v", err)
		return
	}
	if header[0] != 19 {
		err = fmt.Errorf("First byte is not 19")
		return
	}
	_, err = conn.Read(header[1:20])
	if err != nil {
		err = fmt.Errorf("Couldn't read magic string: %v", err)
		return
	}
	if string(header[1:20]) != "BitTorrent protocol" {
		err = fmt.Errorf("Magic string is not correct: %v", string(header[1:20]))
		return
	}
	// Read rest of header
	_, err = conn.Read(header[20:])
	if err != nil {
		err = fmt.Errorf("Couldn't read rest of header")
		return
	}

	h = make([]byte, 48)
	copy(h, header[20:])
	return
}
