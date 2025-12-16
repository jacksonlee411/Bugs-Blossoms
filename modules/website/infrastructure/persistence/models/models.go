package models

import (
	"time"
)

type AIChatConfig struct {
	ID           string
	TenantID     string
	ModelName    string
	ModelType    string
	SystemPrompt string
	Temperature  float32
	MaxTokens    int
	BaseURL      string
	AccessToken  string
	IsDefault    bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ChatThread struct {
	ID        string              `json:"id"`
	TenantID  string              `json:"tenant_id"`
	Phone     string              `json:"phone"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	Messages  []ChatThreadMessage `json:"messages"`
}

type ChatThreadMessage struct {
	Role      string    `json:"role"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}
