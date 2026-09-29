package file

import (
	"time"
)

var defaultTargets = []Target{
	{
		Name:    "Google",
		URL:     "https://www.google.com",
		Enabled: true,
		Timeout: 5 * time.Second,
		ExpectedStatus: []int{200,201,204},
	},
	{
		Name:    "GitHub",
		URL:     "https://github.com",
		Enabled: true,
		Timeout: 5 * time.Second,
		ExpectedStatus: []int{200,201,204},
	},
}

type TargetLoader interface {
	Load() ([]Target, error)
}

type Target struct {
	Name    string        `json:"name" yaml:"name"`
	URL     string        `json:"url" yaml:"url"`
	Enabled bool          `json:"enabled" yaml:"enabled"`
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
	ExpectedStatus []int `json:"expected_status" yaml:"expected_status"`
}
