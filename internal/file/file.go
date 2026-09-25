package file

import (
	"errors"
	"os"
)

type FileDefault struct {
	Path string
}

func (f *FileDefault) FileExists() bool {
	_, err := os.Stat(f.Path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
