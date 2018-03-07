package file

type InfoDict struct {
	PieceLength int64 `bencode:"piece length"`
	Pieces      string
	Private     int64
	Name        string
	Files       []FileDict
}

// Copy the non-default values from an InfoDict to a map.
func (i *InfoDict) ToMap() (m map[string]interface{}) {
	id := map[string]interface{}{}
	// InfoDict
	if i.PieceLength != 0 {
		id["piece length"] = i.PieceLength
	}
	if i.Pieces != "" {
		id["pieces"] = i.Pieces
	}
	if i.Private != 0 {
		id["private"] = i.Private
	}
	if i.Name != "" {
		id["name"] = i.Name
	}
	if len(i.Files) > 0 {
		var fi []map[string]interface{}
		for ii := range i.Files {
			f := &i.Files[ii]
			fd := map[string]interface{}{}
			if f.Length > 0 {
				fd["length"] = f.Length
			}
			if len(f.Path) > 0 {
				fd["path"] = f.Path
			}
			if f.SHA1hash != "" {
				fd["md5sum"] = f.SHA1hash
			}
			if len(fd) > 0 {
				fi = append(fi, fd)
			}
		}
		if len(fi) > 0 {
			id["files"] = fi
		}
	}
	if len(id) > 0 {
		m = id
	}
	return
}
