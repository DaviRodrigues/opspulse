package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

/*
TODO: validar depois formas de enviar notificação por outros serviços email, slack e etc..
Além disso, validar de pegar outras informações fora o básico do healthcheck: headers, security, body e validar
serviços tipo banco de dados etc...
Fazer uma forma de ter um checker pra UP constante ou de tempos em tempos altos, o DOWN ainda é o mais importante
*/

type CheckLogs struct {
	ID         bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	TargetID   bson.ObjectID `json:"target_id,omitempty" bson:"target_id,omitempty"`
	Name       string        `json:"name" bson:"name"`
	URL        string        `json:"url" bson:"url"`
	StatusCode int           `json:"status_code" bson:"status_code"`
	Latency    time.Duration `json:"latency" bson:"latency"`
	IsUp       bool          `json:"is_up" bson:"is_up"`
	Error      string        `json:"error,omitempty" bson:"error,omitempty"`
	CheckedAt  time.Time     `json:"checked_at" bson:"checked_at"`
}

type CheckLog = CheckLogs

