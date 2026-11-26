package file

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DaviRodrigues/opspulse/internal/domain"
)

type Loader interface {
	Load() ([]domain.TargetResult, error)
}

type UnmarshalFunc func(data []byte, v any) error
type CreateFunc func() error
type LoaderFactory func(path string) Loader

var loaderFactories = map[string]LoaderFactory{
	"json": func(path string) Loader {
		return &JSONFile{FileDefault: FileDefault{Path: path}}
	},
	"yaml": func(path string) Loader {
		return &YAMLFile{FileDefault: FileDefault{Path: path}}
	},
}

type FileDefault struct {
	Path string
}

func GetLoaderByFile(path string) (Loader, bool) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	factory, exists := loaderFactories[ext]
	if !exists {
		return nil, false
	}

	return factory(path), true
}

func (f *FileDefault) FileExists() bool {
	_, err := os.Stat(f.Path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func (f *FileDefault) Validate(targets []domain.TargetResult) error {
	if len(targets) == 0 {
		return errors.New("o arquivo de targets está vazio ou não possui nenhum serviço configurado")
	}

	var wg sync.WaitGroup
	errorsChan := make(chan error, len(targets))
	for idx, t := range targets {
		wg.Go(func() {
			if t.URL == "" {
				errorsChan <- fmt.Errorf("target no índice %d não possui 'url'", idx)
				return
			}

			parsedURL, err := url.ParseRequestURI(t.URL)
			if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
				errorsChan <- fmt.Errorf("target '%s' possui URL inválida: %s", t.Name, t.URL)
			}

			if len(targets[idx].ExpectedStatus) <= 0 {
				targets[idx].ExpectedStatus = []int{200, 201, 204}
			}
		})
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

func (f *FileDefault) Load(createFn CreateFunc, unmarshalFn UnmarshalFunc) ([]domain.TargetResult, error) {
	if !f.FileExists() {
		if err := createFn(); err != nil {
			return nil, err
		}
	}
	bytes, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo %s: %w", f.Path, err)
	}

	var targets []domain.TargetResult
	if err := unmarshalFn(bytes, &targets); err != nil {
		return nil, fmt.Errorf("erro de sintaxe no arquivo %s: %w", f.Path, err)
	}

	if err := f.Validate(targets); err != nil {
		return nil, fmt.Errorf("validação falhou em %s: %w", f.Path, err)
	}

	return targets, nil
}
