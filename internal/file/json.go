package file

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DaviRodrigues/opspulse/internal/domain"
)

type JSONFile struct {
	FileDefault
}

func (j *JSONFile) Create() error {
	data, err := json.MarshalIndent(domain.DefaultTargets, "", "  ")
	if err != nil {
		return fmt.Errorf("falha ao serializar targets padrão: %w", err)
	}

	if dir := filepath.Dir(j.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("falha ao criar diretório %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(j.Path, data, 0644); err != nil {
		return fmt.Errorf("falha ao criar arquivo %s: %w", j.Path, err)
	}

	return nil
}

func (j *JSONFile) Load() ([]domain.TargetResult, error) {
	return j.FileDefault.Load(j.Create, json.Unmarshal)
}
