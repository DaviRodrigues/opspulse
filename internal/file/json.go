package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
)

type JSONFile struct {
	FileDefault
}

func (j *JSONFile) Create() error {
	data, err := json.MarshalIndent(defaultTargets, "", "  ")
	if err != nil {
		return fmt.Errorf("falha ao serializar targets padrão: %w", err)
	}

	if err := os.WriteFile(j.FullPath(), data, 0644); err != nil {
		return fmt.Errorf("falha ao criar arquivo %s: %w", j.FullPath(), err)
	}

	return nil
}

// TODO preciso validar se vai valer a pena usar o validate, pois vai ficar muito parecido em outros arquivos
func (j *JSONFile) Validate(targets []Target) error {
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

func (j *JSONFile) Load() ([]Target, error) {
	if !j.FileExists() {
		if err := j.Create(); err != nil {
			return nil, err
		}
	}

	bytes, err := os.ReadFile(j.FullPath())
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo %s: %w", j.FullPath(), err)
	}

	var targets []Target
	if err := json.Unmarshal(bytes, &targets); err != nil {
		return nil, fmt.Errorf("erro de sintaxe no JSON em %s: %w", j.FullPath(), err)
	}

	if err := j.Validate(targets); err != nil {
		return nil, fmt.Errorf("validação falhou em %s: %w", j.FullPath(), err)
	}

	return targets, nil
}
