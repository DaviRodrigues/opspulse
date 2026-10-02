package domain

type Event struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Data  any    `json:"data"`
	Retry int    `json:"retry,omitempty"`
}