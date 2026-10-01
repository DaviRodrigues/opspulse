package file

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

type YAMLFile struct {
	FileDefault
}

func (y *YAMLFile) Create() error {
	data, err := yaml.Marshal(defaultTargets)
	if err != nil {
		return fmt.Errorf("falha ao serializar targets padrão: %w", err)
	}

	if err := os.WriteFile(y.Path, data, 0644); err != nil {
		return fmt.Errorf("falha ao criar arquivo %s: %w", y.Path, err)
	}

	return nil
}

func (y *YAMLFile) Load() ([]Target, error) {
	return y.FileDefault.Load(y.Create, yaml.Unmarshal)
}
