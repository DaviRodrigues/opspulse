package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var DefaultTargets = []Target{
	{
		Name:           "Google",
		URL:            "https://www.google.com",
		Enabled:        true,
		Timeout:        5 * time.Second,
		ExpectedStatus: []int{200, 201, 204},
	},
	{
		Name:           "GitHub",
		URL:            "https://github.com",
		Enabled:        true,
		Timeout:        5 * time.Second,
		ExpectedStatus: []int{200, 201, 204},
	},
}

type Target struct {
	ID                bson.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name              string         `json:"name" yaml:"name" bson:"name"`
	URL               string         `json:"url" yaml:"url" bson:"url"`
	Method            string         `json:"method" yaml:"method" bson:"method"`
	Enabled           bool           `json:"enabled" yaml:"enabled"`
	Tags              []string       `json:"tags" yaml:"tags" bson:"tags"`
	Body              map[string]any `json:"body" yaml:"body" bson:"body"`
	Timeout           time.Duration  `json:"timeout" yaml:"timeout" bson:"timeout"`
	ExpectedStatus    []int          `json:"expected_status" yaml:"expected_status" bson:"expected_status"`
	ExpectedBodyMatch string         `json:"expected_body_match" yaml:"expected_body_match" bson:"expected_body_match"`
	CreatedAt         time.Time      `json:"created_at" yaml:"created_at" bson:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at" yaml:"updated_at" bson:"updated_at"`
}
