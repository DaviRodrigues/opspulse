package file

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

type YAMLFile struct {
	FileDefault
}

func (y *YAMLFile) Create() error {
	data, err := yaml.Marshal(defaultTargets)
	if err != nil {
		return fmt.Errorf("falha ao serializar targets padrão: %w", err)
	}

	if err := os.WriteFile(y.FullPath(), data, 0644); err != nil {
		return fmt.Errorf("falha ao criar arquivo %s: %w", y.FullPath(), err)
	}

	return nil
}

func (y *YAMLFile) Validate(targets []Target) error {
	if len(targets) == 0 {
		return errors.New("o arquivo de targets está vazio ou não possui nenhum serviço configurado")
	}

	var wg sync.WaitGroup
	errorsChan := make(chan error, len(targets))
	for idx, t := range targets {
		wg.Add(1)

		go func(idx int, t Target) {
			defer wg.Done()
			if t.URL == "" {
				errorsChan <- fmt.Errorf("target no índice %d não possui 'url'", idx)
				return
			}

			parsedURL, err := url.ParseRequestURI(t.URL)
			if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
				errorsChan <- fmt.Errorf("target '%s' possui URL inválida: %s", t.Name, t.URL)
			}
		}(idx, t)
	}

	wg.Wait()
	close(errorsChan)

	if len(errorsChan) == 0 {
		return nil
	}

	var errs []error
	for err := range errorsChan {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (y *YAMLFile) Load() ([]Target, error) {
	if !y.FileExists() {
		if err := y.Create(); err != nil {
			return nil, err
		}
	}

	bytes, err := os.ReadFile(y.FullPath())
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo %s: %w", y.FullPath(), err)
	}

	var targets []Target
	if err := yaml.Unmarshal(bytes, &targets); err != nil {
		return nil, fmt.Errorf("erro de sintaxe no YAML em %s: %w", y.FullPath(), err)
	}

	if err := y.Validate(targets); err != nil {
		return nil, fmt.Errorf("validação falhou em %s: %w", y.FullPath(), err)
	}

	return targets, nil
}
