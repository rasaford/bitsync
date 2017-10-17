package meta

import (
	"log"
	"path/filepath"
	"regexp"
)

// ExclusionPattern defines a slice of patterns that will be excluded when filtering the
// files to add to the .torrent:
type ExclusionPattern struct {
	patterns []string
}

func DefaultExPattern() *ExclusionPattern {
	e := &ExclusionPattern{}
	// ignore the .bitsync file and all hidden folders
	e.Add("\\w*.bitsync",
		"^\\.\\w*",
		".*\\s.*",
	)
	return e
}

// Matches retruns true if any of the stored patterns match the string
func (e *ExclusionPattern) Matches(path string) bool {
	for _, pattern := range e.patterns {
		res, err := regexp.MatchString(pattern, filepath.Clean(path))
		if err != nil {
			log.Printf("cannot match against pattern %s\n", pattern)
			continue
		}
		if res {
			log.Printf("excluded: %s\n", path)
			return true
		}
	}
	return false
}

// Add adds a regex pattern to the list op patterns to match against
func (e *ExclusionPattern) Add(pattern ...string) {
	for _, p := range pattern {
		e.patterns = append(e.patterns, p)
	}
}
