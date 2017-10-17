package meta

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/proxy"

	bencode "github.com/jackpal/bencode-go"
	"github.com/nictuku/dht"
	"github.com/pkg/errors"
	"github.com/rasaford/bitsync/torrent/file"
	"github.com/rasaford/bitsync/torrent/hash"
	tproxy "github.com/rasaford/bitsync/torrent/proxy"
	"github.com/rasaford/bitsync/torrent/uri"
)

type MetaInfo struct {
	Info         file.InfoDict
	InfoHash     string
	Announce     string
	AnnounceList [][]string `bencode:"announce-list"`
	CreationDate string     `bencode:"creation date"`
	Comment      string
	CreatedBy    string `bencode:"created by"`
	Encoding     string
}

func getString(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Parse a list of list of strings structure, filtering out anything that's
// not a string, and filtering out empty lists. May return nil.
func getSliceSliceString(m map[string]interface{}, k string) (aas [][]string) {
	if a, ok := m[k]; ok {
		if b, ok := a.([]interface{}); ok {
			for _, c := range b {
				if d, ok := c.([]interface{}); ok {
					var sliceOfStrings []string
					for _, e := range d {
						if f, ok := e.(string); ok {
							sliceOfStrings = append(sliceOfStrings, f)
						}
					}
					if len(sliceOfStrings) > 0 {
						aas = append(aas, sliceOfStrings)
					}
				}
			}
		}
	}
	return
}

func GetMetaInfo(dialer proxy.Dialer, torrent string) (metaInfo *MetaInfo, err error) {
	var input io.ReadCloser
	if strings.HasPrefix(torrent, "http:") {
		r, err := tproxy.HttpGet(dialer, torrent)
		if err != nil {
			return nil, err
		}
		input = r.Body
	} else if strings.HasPrefix(torrent, "magnet:") {
		magnet, err := uri.ParseMagnet(torrent)
		if err != nil {
			log.Println("Couldn't parse magnet: ", err)
			return nil, err
		}

		ih, err := dht.DecodeInfoHash(magnet.InfoHashes[0])
		if err != nil {
			return nil, err
		}

		metaInfo = &MetaInfo{InfoHash: string(ih), AnnounceList: magnet.Trackers}

		//Gives us something to call the torrent until metadata can be procurred
		metaInfo.Info.Name = hex.EncodeToString([]byte(ih))

		return metaInfo, err

	} else {
		if input, err = os.Open(torrent); err != nil {
			return
		}
	}

	// We need to calcuate the sha1 of the Info map, including every value in the
	// map. The easiest way to do this is to read the data using the Decode
	// API, and then pick through it manually.
	var m interface{}
	m, err = bencode.Decode(input)
	input.Close()
	if err != nil {
		err = errors.Wrap(err, "Couldn't parse torrent file phase 1")
		return
	}

	topMap, ok := m.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("couldn't parse torrent file phase 2")
		return
	}

	infoMap, ok := topMap["info"]
	if !ok {
		err = fmt.Errorf("Couldn't parse torrent file. info")
		return
	}
	var b bytes.Buffer
	if err = bencode.Marshal(&b, infoMap); err != nil {
		return
	}
	hash := sha1.New()
	hash.Write(b.Bytes())

	var m2 MetaInfo
	err = bencode.Unmarshal(&b, &m2.Info)
	if err != nil {
		return
	}

	m2.InfoHash = string(hash.Sum(nil))
	m2.Announce = getString(topMap, "announce")
	m2.AnnounceList = getSliceSliceString(topMap, "announce-list")
	m2.CreationDate = getString(topMap, "creation date")
	m2.Comment = getString(topMap, "comment")
	m2.CreatedBy = getString(topMap, "created by")
	m2.Encoding = strings.ToUpper(getString(topMap, "encoding"))

	metaInfo = &m2
	return
}

type MetaInfoFileSystem interface {
	Open(name string) (MetaInfoFile, error)
	Stat(name string) (os.FileInfo, error)
}

type MetaInfoFile interface {
	io.Closer
	io.Reader
	io.ReaderAt
	Readdirnames(n int) (names []string, err error)
	Stat() (os.FileInfo, error)
}

type OSMetaInfoFileSystem struct {
	dir string
}

func (o *OSMetaInfoFileSystem) Open(name string) (MetaInfoFile, error) {
	return os.Open(path.Join(o.dir, name))
}

func (o *OSMetaInfoFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(path.Join(o.dir, name))
}

// Adapt a MetaInfoFileSystem into a torrent file store FileSystem
type FileStoreFileSystemAdapter struct {
	m MetaInfoFileSystem
}

type FileStoreFileAdapter struct {
	f MetaInfoFile
}

func (f *FileStoreFileSystemAdapter) Open(file *file.FileDict) (file.File, error) {
	ff, err := f.m.Open(path.Join(file.Path...))
	if err != nil {
		return nil, err
	}
	stat, err := ff.Stat()
	if err != nil {
		return nil, err
	}
	actualSize := stat.Size()
	if actualSize != file.Length {
		err := fmt.Errorf("Unexpected file size %v. Expected %v", actualSize, file.Length)
		return nil, err
	}
	return &FileStoreFileAdapter{ff}, nil
}

func (f *FileStoreFileSystemAdapter) Close() error {
	return nil
}

func (f *FileStoreFileAdapter) ReadAt(p []byte, off int64) (n int, err error) {
	return f.f.ReadAt(p, off)
}

func (f *FileStoreFileAdapter) WriteAt(p []byte, off int64) (n int, err error) {
	// Writes must match existing data exactly.
	q := make([]byte, len(p))
	_, err = f.ReadAt(q, off)
	if err != nil {
		return
	}
	if bytes.Compare(p, q) != 0 {
		err = fmt.Errorf("new data does not match original data")
	}
	return
}

func (f *FileStoreFileAdapter) Close() (err error) {
	return f.f.Close()
}

// Create a MetaInfo for a given file and file system.
// If fs is nil then the OSMetaInfoFileSystem will be used.
// If pieceLength is 0 then an optimal piece length will be chosen.
func Index(fs MetaInfoFileSystem, dir, tracker string, pieceSize int64) (*MetaInfo, error) {
	if fs == nil {
		r, f := filepath.Split(dir)
		fs = &OSMetaInfoFileSystem{r}
		dir = f
	}
	info := &MetaInfo{}
	exclusion := DefaultExPattern()
	var totalLength int64
	if err := info.addFiles(fs, dir, exclusion); err != nil {
		return nil, err
	}
	for _, fd := range info.Info.Files { // calc total length of all added files
		totalLength += fd.Length
	}

	if pieceSize == 0 {
		pieceSize = choosePieceLength(totalLength)
	}
	info.Info.PieceLength = pieceSize
	fileStoreFS := &FileStoreFileSystemAdapter{fs}
	fileStore, fsLength, err := file.NewFileStore(&info.Info, fileStoreFS)
	if err != nil {
		return nil, err
	}
	if fsLength != totalLength {
		err = fmt.Errorf("Filestore total length %v, expected %v", fsLength, totalLength)
		return nil, err
	}
	sums, err := computeSums(fileStore, totalLength, pieceSize)
	if err != nil {
		return nil, err
	}
	info.Info.Pieces = string(sums)
	// m.UpdateInfoHash(nil)
	if tracker != "" {
		info.Announce = fmt.Sprintf("http://%s/announce", tracker)
	}
	return info, err
}

const MinimumPieceLength = 16 * 1024
const TargetPieceCountLog2 = 10
const TargetPieceCountMin = 1 << TargetPieceCountLog2

// Target piece count should be < TargetPieceCountMax
const TargetPieceCountMax = TargetPieceCountMin << 1

// Choose a good piecelength.
func choosePieceLength(totalLength int64) int64 {
	// Must be a power of 2.
	// Must be a multiple of 16KB
	// Prefer to provide around 1024..2048 pieces.
	var pieceLength int64 = MinimumPieceLength
	pieces := totalLength / pieceLength
	for pieces >= TargetPieceCountMax {
		pieceLength <<= 1
		pieces >>= 1
	}
	return pieceLength
}

func roundUpToPowerOfTwo(v uint64) uint64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++
	return v
}

func WriteMetaInfoBytes(root, tracker string, w io.Writer) (err error) {
	var m *MetaInfo
	m, err = Index(nil, root, tracker, 0)
	if err != nil {
		return
	}
	// log.Printf("Metainfo: %#v", m)
	err = m.Bencode(w)
	if err != nil {
		return
	}
	return
}

// addFiles adds all the files below if it does not mathe any of the exclusion patterns.
func (m *MetaInfo) addFiles(fileSystem MetaInfoFileSystem, dir string, exclusion *ExclusionPattern) error {
	fileInfo, err := fileSystem.Stat(dir)
	if err != nil {
		return err
	}
	if fileInfo.IsDir() { // if is dir recurse
		f, err := fileSystem.Open(dir)
		if err != nil {
			return err
		}
		files, err := f.Readdirnames(0)
		if err != nil {
			return err
		}
		for _, file := range files {
			// recursively add all files withing the search dir
			if err := m.addFiles(fileSystem, filepath.Join(dir, file), exclusion); err != nil {
				return err
			}
		}
	} else if !exclusion.Matches(dir) { // only add the file if it does not match any of the exclusion patterns
		fd, err := fileSystem.Open(dir)
		if err != nil {
			return errors.Wrap(err, "open failed")
		}
		md5 := hash.FileHash(fd)
		fileDict := file.FileDict{
			Length: fileInfo.Size(),
			Path:   strings.Split(filepath.Clean(dir), string(os.PathSeparator)),
			MD5Sum: md5,
		}
		m.Info.Files = append(m.Info.Files, fileDict)
	}
	return nil
}

// Updates the InfoHash field. Call this after manually changing the Info data.
func (m *MetaInfo) UpdateInfoHash(metaInfo *MetaInfo) (err error) {
	var b bytes.Buffer
	infoMap := m.Info.ToMap()
	if len(infoMap) > 0 {
		err = bencode.Marshal(&b, infoMap)
		if err != nil {
			return
		}
	}
	hash := sha1.New()
	hash.Write(b.Bytes())

	m.InfoHash = string(hash.Sum(nil))
	return
}

// Encode to Bencode, but only encode non-default values.
func (m *MetaInfo) Bencode(w io.Writer) (err error) {
	var mi map[string]interface{} = map[string]interface{}{}
	id := m.Info.ToMap()
	if len(id) > 0 {
		mi["info"] = id
	}
	// Do not encode InfoHash. Clients are supposed to calculate it themselves.
	if m.Announce != "" {
		mi["announce"] = m.Announce
	}
	if len(m.AnnounceList) > 0 {
		mi["announce-list"] = m.AnnounceList
	}
	if m.CreationDate != "" {
		mi["creation date"] = m.CreationDate
	}
	if m.Comment != "" {
		mi["comment"] = m.Comment
	}
	if m.CreatedBy != "" {
		mi["created by"] = m.CreatedBy
	}
	if m.Encoding != "" {
		mi["encoding"] = m.Encoding
	}
	bencode.Marshal(w, mi)
	return
}

type TrackerResponse struct {
	FailureReason  string `bencode:"failure reason"`
	WarningMessage string `bencode:"warning message"`
	Interval       uint
	MinInterval    uint   `bencode:"min interval"`
	TrackerId      string `bencode:"tracker id"`
	Complete       uint
	Incomplete     uint
	Peers          string
	Peers6         string
}

type SessionInfo struct {
	PeerID       string
	Port         uint16
	OurAddresses map[string]bool //List of addresses that resolve to ourselves.
	Uploaded     uint64
	Downloaded   uint64
	Left         uint64

	UseDHT      bool
	FromMagnet  bool
	HaveTorrent bool

	OurExtensions map[int]string
	ME            *MetaDataExchange
}

type MetaDataExchange struct {
	Transferring bool
	Pieces       [][]byte
}

func TrackerInfo(dialer proxy.Dialer, url string) (tr *TrackerResponse, err error) {
	r, err := tproxy.HttpGet(dialer, url)
	if err != nil {
		return
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		data, _ := ioutil.ReadAll(r.Body)
		reason := "Bad Request " + string(data)
		log.Println(reason)
		err = fmt.Errorf(reason)
		return
	}
	var tr2 TrackerResponse
	err = bencode.Unmarshal(r.Body, &tr2)
	r.Body.Close()
	if err != nil {
		return
	}
	tr = &tr2
	return
}

func SaveMetaInfo(metadata string) (err error) {
	var info file.InfoDict
	err = bencode.Unmarshal(bytes.NewReader([]byte(metadata)), &info)
	if err != nil {
		return
	}

	f, err := os.Create(info.Name + ".torrent")
	if err != nil {
		log.Println("Error when opening file for creation: ", err)
		return
	}
	defer f.Close()

	_, err = f.WriteString(metadata)

	return
}
