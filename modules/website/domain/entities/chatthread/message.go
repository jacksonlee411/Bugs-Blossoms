package chatthread

import (
	"errors"
	"time"
)

var (
	ErrInvalidRole = errors.New("invalid role")
)

type Message interface {
	Role() Role
	Message() string
	Timestamp() time.Time
}

type message struct {
	role      Role
	message   string
	timestamp time.Time
}

func NewMessage(role Role, text string, timestamp time.Time) (Message, error) {
	if text == "" {
		return nil, ErrEmptyMessage
	}
	if len(text) > MaxMessageLength {
		return nil, ErrMessageTooLong
	}
	switch role {
	case RoleUser, RoleAssistant:
	default:
		return nil, ErrInvalidRole
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	return &message{
		role:      role,
		message:   text,
		timestamp: timestamp,
	}, nil
}

func MustNewMessage(role Role, text string, timestamp time.Time) Message {
	msg, err := NewMessage(role, text, timestamp)
	if err != nil {
		panic(err)
	}
	return msg
}

func (m *message) Role() Role {
	return m.role
}

func (m *message) Message() string {
	return m.message
}

func (m *message) Timestamp() time.Time {
	return m.timestamp
}
