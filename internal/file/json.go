package file

import (
	"encoding/json"
	"fmt"
	"os"
)

type JSONFile struct {
	FileDefault
}

func (j *JSONFile) Create() error {
	data, err := json.MarshalIndent(defaultTargets, "", "  ")
	if err != nil {
		return fmt.Errorf("falha ao serializar targets padrão: %w", err)
	}

	if err := os.WriteFile(j.Path, data, 0644); err != nil {
		return fmt.Errorf("falha ao criar arquivo %s: %w", j.Path, err)
	}

	return nil
}

func (j *JSONFile) Load() ([]Target, error) {
	return j.FileDefault.Load(j.Create, json.Unmarshal)
}
