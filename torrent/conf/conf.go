package conf

import (
	"github.com/rasaford/bitsync/torrent/cache"
	"github.com/rasaford/bitsync/torrent/file"
	"golang.org/x/net/proxy"
)

const (
	MAX_NUM_PEERS    = 60
	TARGET_NUM_PEERS = 15
)

// BitTorrent message types. Sources:
// http://bittorrent.org/beps/bep_0003.html
// http://wiki.theory.org/BitTorrentSpecification
const (
	CHOKE = iota
	UNCHOKE
	INTERESTED
	NOT_INTERESTED
	HAVE
	BITFIELD
	REQUEST
	PIECE
	CANCEL
	PORT      // Not implemented. For DHT support.
	EXTENSION = 20
)

const (
	EXTENSION_HANDSHAKE = iota
)

const (
	METADATA_REQUEST = iota
	METADATA_DATA
	METADATA_REJECT
)

type Flags struct {
	Port                int
	FileDir             string
	SeedRatio           float64
	UseDeadlockDetector bool
	UseLPD              bool
	UseDHT              bool
	UseUPnP             bool
	UseNATPMP           bool
	TrackerlessMode     bool
	ExecOnSeeding       string

	// The dial function to use. Nil means use net.Dial
	Dial proxy.Dialer

	// IP address of gateway used for NAT-PMP
	Gateway string

	//Provides the filesystems added torrents are saved to
	FileSystemProvider file.FsProvider

	//Whether to check file hashes when adding torrents
	InitialCheck bool

	//Provides cache to each torrent
	Cacher cache.Provider

	//Whether to write and use *.haveBitset resume data
	QuickResume bool

	//How many torrents should be active at a time
	MaxActive int

	//Maximum amount of memory (in MiB) to use for each torrent's Active Pieces.
	//0 means a single Active Piece. Negative means Unlimited Active Pieces.
	MemoryPerTorrent int
}
