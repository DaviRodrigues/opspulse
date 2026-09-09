package config

import (
	"os"
	"time"
)

type TargetLoader interface {
	LoadTargets() ([]Target, error)
}

type Target struct {
	Name    string        `json:"name" yaml:"name"`
	URL     string        `json:"url" yaml:"url"`
	Enabled bool          `json:"enabled" yaml:"enabled"`
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}

type JSONTargetLoader struct{}

type YAMLTargetLoader struct{}

type EnvTargetLoader struct{}

func (j *JSONTargetLoader) LoadTargets() ([]Target, error) {
	return []Target{}, nil
}

func (j *JSONTargetLoader) validate() error {
	return nil
}

func (y *YAMLTargetLoader) LoadTargets() ([]Target, error) {
	return []Target{}, nil
}

func (y *YAMLTargetLoader) validate() error {
	

	return nil
}

func (e *EnvTargetLoader) LoadTargets() ([]Target, error) {
	urls, err := loadListEnv("TARGET_URLS")
	if err != nil {
		return nil, err
	}

	checkTimeout, err := loadDurationEnv("CHECK_TIMEOUT")
	if err != nil {
		return nil, err
	}

	var targetUrls []Target
	for _, url := range urls {
		targetUrls = append(targetUrls, Target{
			URL:     url,
			Enabled: true,
			Timeout: checkTimeout,
		})
	}

	return targetUrls, nil
}