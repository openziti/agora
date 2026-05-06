//go:build no_agora_ui

package ui

import (
	"io/fs"
	"os"
)

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) {
	return nil, os.ErrNotExist
}

func distFS() fs.FS {
	return emptyFS{}
}
