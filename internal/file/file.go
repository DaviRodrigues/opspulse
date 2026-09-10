package file

import (
	"errors"
	"os"
	"path/filepath"
)

type FileDefault struct {
	Name string
	Path string
}

func (f *FileDefault) FullPath() string {
	if f.Path == "" {
		return f.Name
	}
	return filepath.Join(f.Path, f.Name)
}

func (f *FileDefault) FileExists() bool {
	_, err := os.Stat(f.FullPath())
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
