package file

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/DaviRodrigues/opspulse/internal/config"
)

type FileManager struct {
	Name string
	Path string
}

func (f *FileManager) exists() bool {
	_, err := os.Stat(f.Name)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}

	return false
}

func (f *FileManager) create() {
	if f.exists() {
		return
	}

	file, err := os.Create(f.Name)
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(config.Target{})
	if err != nil {
		return
	}
}