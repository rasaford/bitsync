package choker

import (
	"math/rand"
	"sort"
)

const HighBandwidthSlots = 3
const OptimisticUnchokeIndex = HighBandwidthSlots

// How many cycles of this algorithm before we pick a new optimistic
const OptimisticUnchokeCount = 3

// BitTorrent choking policy.

// The choking policy's view of a peer. For current policies we only care
// about identity and download bandwidth.
type Choker interface {
	DownloadBPS() float32 // bps
}

type ChokePolicy interface {
	// Only pass in interested peers.
	// mutate the chokers into a list where the first N are to be unchoked.
	Choke(chokers []Choker) (unchokeCount int, err error)
}

// Our naive never-choke policy
type NeverChokePolicy struct{}

func (n *NeverChokePolicy) Choke(chokers []Choker) (unchokeCount int, err error) {
	return len(chokers), nil
}

// Our interpretation of the classic bittorrent choke policy.
// Expects to be called once every 10 seconds.
// See the section "Choking and optimistic unchoking" in
// https://wiki.theory.org/BitTorrentSpecification
type ClassicChokePolicy struct {
	optimisticUnchoker Choker // The choker we unchoked optimistically
	counter            int    // When to choose a new optimisticUnchoker.
}

type ByDownloadBPS []Choker

func (a ByDownloadBPS) Len() int {
	return len(a)
}

func (a ByDownloadBPS) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a ByDownloadBPS) Less(i, j int) bool {
	return a[i].DownloadBPS() > a[j].DownloadBPS()
}

func (ccp *ClassicChokePolicy) Choke(chokers []Choker) (unchokeCount int, err error) {
	sort.Sort(ByDownloadBPS(chokers))

	optimistIndex := ccp.findOptimist(chokers)
	if optimistIndex >= 0 {
		if optimistIndex < OptimisticUnchokeIndex {
			// Forget optimistic choke
			optimistIndex = -1
		} else {
			ByDownloadBPS(chokers).Swap(OptimisticUnchokeIndex, optimistIndex)
			optimistIndex = OptimisticUnchokeIndex
		}
	}

	if optimistIndex >= 0 {
		ccp.counter++
		if ccp.counter >= OptimisticUnchokeCount {
			ccp.counter = 0
			optimistIndex = -1
		}
	}

	if optimistIndex < 0 {
		candidateCount := len(chokers) - OptimisticUnchokeIndex
		if candidateCount > 0 {
			candidate := OptimisticUnchokeIndex + rand.Intn(candidateCount)
			ByDownloadBPS(chokers).Swap(OptimisticUnchokeIndex, candidate)
			ccp.counter = 0
			ccp.optimisticUnchoker = chokers[OptimisticUnchokeIndex]
		}
	}
	unchokeCount = OptimisticUnchokeIndex + 1
	if unchokeCount > len(chokers) {
		unchokeCount = len(chokers)
	}
	return
}

func (ccp *ClassicChokePolicy) findOptimist(chokers []Choker) (index int) {
	for i, c := range chokers {
		if c == ccp.optimisticUnchoker {
			return i
		}
	}
	return -1
}
