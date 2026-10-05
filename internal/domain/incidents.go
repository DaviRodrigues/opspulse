package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Incident struct {
	ID              bson.ObjectID `json:"id" bson:"_id,omitempty"`
	TargetID        bson.ObjectID `json:"target_id" bson:"target_id"`
	URL             string        `json:"url" bson:"url"`
	Status          string        `json:"status" bson:"status"` // "OPEN" | "RESOLVED"
	StartedAt       time.Time     `json:"started_at" bson:"started_at"`
	ResolvedAt      *time.Time    `json:"resolved_at,omitempty" bson:"resolved_at,omitempty"`
	DurationSeconds int64         `json:"duration_seconds,omitempty" bson:"duration_seconds,omitempty"`
	RootCause       string        `json:"root_cause,omitempty" bson:"root_cause,omitempty"`
}

type Incidents = Incident
