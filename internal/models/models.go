package models

import "time"

type Conversation struct {
	ID        string     `json:"id"`
	NodeID    string     `json:"node_id"`
	Command   string     `json:"command"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type Message struct {
	ID             int64     `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Sequence       int       `json:"sequence"`
	CreatedAt      time.Time `json:"created_at"`
}

type LoomInfo struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Command        string    `json:"command"`
	Address        string    `json:"address"`
	StartedAt      time.Time `json:"started_at"`
}

type RegisterRequest struct {
	Loom LoomInfo `json:"loom"`
}

type InjectRequest struct {
	Message string `json:"message"`
}

type ResizeRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}
