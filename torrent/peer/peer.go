package peer

import (
	"bytes"
	"io"
	"log"
	"net"
	"time"

	bencode "github.com/jackpal/bencode-go"
	"github.com/rasaford/bitsync/torrent/accumulator"
	"github.com/rasaford/bitsync/torrent/bitset"
	"github.com/rasaford/bitsync/torrent/conf"
	"github.com/rasaford/bitsync/torrent/convert"
)

const MAX_OUR_REQUESTS = 2
const MAX_PEER_REQUESTS = 10
const STANDARD_BLOCK_LENGTH = 16 * 1024

type PeerMessage struct {
	Peer    *PeerState
	Message []byte // nil means an error occurred
}

type PeerState struct {
	Address        string
	ID             string
	writeChan      chan []byte
	writeChan2     chan []byte
	LastReadTime   time.Time
	Have           *bitset.Bitset // What the peer has told us it has
	conn           net.Conn
	AmChoking      bool // this client is choking the peer
	AmInterested   bool // this client is interested in the peer
	PeerChoking    bool // peer is choking this client
	PeerInterested bool // peer is interested in this client
	PeerRequests   map[uint64]bool
	OurRequests    map[uint64]time.Time // What we requested, when we requested it

	// This field tells if the peer can send a bitfield or not
	CanReceive      bool
	TheirExtensions map[string]int
	downloaded      accumulator.Accumulator
}

func (p *PeerState) CreditDownload(length int64) {
	p.downloaded.Add(time.Now(), length)
}

func (p *PeerState) ComputeDownloadRate() {
	// Has the side effect of computing the download rate.
	p.downloaded.GetRate(time.Now())
}

func (p *PeerState) DownloadBPS() float32 {
	return float32(p.downloaded.GetRateNoUpdate())
}

func queueingWriter(in, out chan []byte) {
	queue := make(map[int][]byte)
	head, tail := 0, 0
L:
	for {
		if head == tail {
			select {
			case m, ok := <-in:
				if !ok {
					break L
				}
				queue[head] = m
				head++
			}
		} else {
			select {
			case m, ok := <-in:
				if !ok {
					break L
				}
				queue[head] = m
				head++
			case out <- queue[tail]:
				delete(queue, tail)
				tail++
			}
		}
	}
	// We throw away any messages waiting to be sent, including the
	// nil message that is automatically sent when the in channel is closed
	close(out)
}

func NewPeerState(conn net.Conn) *PeerState {
	writeChan := make(chan []byte)
	writeChan2 := make(chan []byte)
	go queueingWriter(writeChan, writeChan2)
	return &PeerState{writeChan: writeChan, writeChan2: writeChan2, conn: conn,
		AmChoking:    true,
		PeerChoking:  true,
		PeerRequests: make(map[uint64]bool, MAX_PEER_REQUESTS),
		OurRequests:  make(map[uint64]time.Time, MAX_OUR_REQUESTS),
		CanReceive:   true}
}

func (p *PeerState) Close() {
	//log.Println("Closing connection to", p.address)
	p.conn.Close()
	// No need to close p.writeChan. Further writes to p.conn will just fail.
}

func (p *PeerState) AddRequest(index, begin, length uint32) {
	if !p.AmChoking && len(p.PeerRequests) < MAX_PEER_REQUESTS {
		offset := (uint64(index) << 32) | uint64(begin)
		p.PeerRequests[offset] = true
	}
}

func (p *PeerState) CancelRequest(index, begin, length uint32) {
	offset := (uint64(index) << 32) | uint64(begin)
	if _, ok := p.PeerRequests[offset]; ok {
		delete(p.PeerRequests, offset)
	}
}

func (p *PeerState) RemoveRequest() (index, begin, length uint32, ok bool) {
	for k, _ := range p.PeerRequests {
		index, begin = uint32(k>>32), uint32(k)
		length = STANDARD_BLOCK_LENGTH
		ok = true
		return
	}
	return
}

func (p *PeerState) SetChoke(choke bool) {
	if choke != p.AmChoking {
		p.AmChoking = choke
		b := byte(conf.UNCHOKE)
		if choke {
			b = conf.CHOKE
			p.PeerRequests = make(map[uint64]bool, MAX_PEER_REQUESTS)
		}
		p.sendOneCharMessage(b)
	}
}

func (p *PeerState) SetInterested(interested bool) {
	if interested != p.AmInterested {
		// log.Println("SetInterested", interested, p.address)
		p.AmInterested = interested
		b := byte(conf.NOT_INTERESTED)
		if interested {
			b = conf.INTERESTED
		}
		p.sendOneCharMessage(b)
	}
}

func (p *PeerState) SendBitfield(bs *bitset.Bitset) {
	msg := make([]byte, len(bs.Bytes())+1)
	msg[0] = conf.BITFIELD
	copy(msg[1:], bs.Bytes())
	p.SendMessage(msg)
}

func (p *PeerState) SendExtensions(port uint16) {
	handshake := map[string]interface{}{
		"m": map[string]int{
			"ut_metadata": 1,
		},
		"v": "Taipei-Torrent dev",
	}

	var buf bytes.Buffer
	err := bencode.Marshal(&buf, handshake)
	if err != nil {
		//log.Println("Error when marshalling extension message")
		return
	}

	msg := make([]byte, 2+buf.Len())
	msg[0] = conf.EXTENSION
	msg[1] = conf.EXTENSION_HANDSHAKE
	copy(msg[2:], buf.Bytes())

	p.SendMessage(msg)
}

func (p *PeerState) sendOneCharMessage(b byte) {
	// log.Println("ocm", b, p.address)
	p.SendMessage([]byte{b})
}

func (p *PeerState) SendMessage(b []byte) {
	p.writeChan <- b
}

func (p *PeerState) KeepAlive(now time.Time) {
	p.SendMessage([]byte{})
}

// There's two goroutines per peer, one to read data from the peer, the other to
// send data to the peer.

func writeNBOUint32(conn net.Conn, n uint32) (err error) {
	var buf []byte = make([]byte, 4)
	convert.Uint32ToBytes(buf, n)
	_, err = conn.Write(buf[0:])
	return
}

func readNBOUint32(conn net.Conn) (n uint32, err error) {
	var buf [4]byte
	_, err = conn.Read(buf[0:])
	if err != nil {
		return
	}
	n = convert.BytesToUint32(buf[0:])
	return
}

// This func is designed to be run as a goroutine. It
// listens for messages on a channel and sends them to a peer.
func (p *PeerState) PeerWriter(errorChan chan PeerMessage) {
	// log.Println("Writing messages")
	var lastWriteTime time.Time

	for msg := range p.writeChan2 {
		now := time.Now()
		if len(msg) == 0 {
			// This is a keep-alive message.
			if now.Sub(lastWriteTime) < 2*time.Minute {
				// Don't need to send keep-alive because we have recently sent a
				// message to this peer.
				continue
			}
			// log.Stderr("Sending keep alive", p)
		}
		lastWriteTime = now

		// log.Println("Writing", uint32(len(msg)), p.conn.RemoteAddr())
		err := writeNBOUint32(p.conn, uint32(len(msg)))
		if err != nil {
			log.Println(err)
			break
		}
		_, err = p.conn.Write(msg)
		if err != nil {
			// log.Println("Failed to write a message", p.address, len(msg), msg, err)
			break
		}
	}
	// log.Println("peerWriter exiting")
	errorChan <- PeerMessage{p, nil}
}

// This func is designed to be run as a goroutine. It
// listens for messages from the peer and forwards them to a channel.
func (p *PeerState) PeerReader(msgChan chan<- PeerMessage) {
	// log.Println("Reading messages")
	for {
		var n uint32
		n, err := readNBOUint32(p.conn)
		if err != nil {
			break
		}
		if n > 130*1024 {
			// log.Println("Message size too large: ", n)
			break
		}

		var buf []byte
		if n == 0 {
			// keep-alive - we want an empty message
			buf = make([]byte, 1)
		} else {
			buf = make([]byte, n)
		}
		_, err = io.ReadFull(p.conn, buf)
		if err != nil {
			break
		}
		msgChan <- PeerMessage{p, buf}
	}

	msgChan <- PeerMessage{p, nil}
	// log.Println("peerReader exiting")
}

func (p *PeerState) SendMetadataRequest(piece int) {
	log.Printf("Sending metadata request for piece %d to %s\n", piece, p.Address)

	m := map[string]int{
		"msg_type": conf.METADATA_REQUEST,
		"piece":    piece,
	}
	var raw bytes.Buffer
	err := bencode.Marshal(&raw, m)
	if err != nil {
		return
	}

	msg := make([]byte, raw.Len()+2)
	msg[0] = conf.EXTENSION
	msg[1] = byte(p.TheirExtensions["ut_metadata"])
	copy(msg[2:], raw.Bytes())

	p.SendMessage(msg)
}
