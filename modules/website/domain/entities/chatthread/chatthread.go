package chatthread

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrChatThreadNotFound = errors.New("chat thread not found")
	ErrEmptyMessage       = errors.New("empty message")
	ErrMessageTooLong     = errors.New("message too long")
	ErrNoMessages         = errors.New("no messages")
)

const (
	MaxMessageLength = 4096
	MaxMessages      = 200
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (ChatThread, error)
	Save(ctx context.Context, thread ChatThread) (ChatThread, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]ChatThread, error)
}

type chatThread struct {
	id        uuid.UUID
	tenantID  uuid.UUID
	phone     string
	createdAt time.Time
	updatedAt time.Time
	messages  []Message
}

type ChatThread interface {
	ID() uuid.UUID
	TenantID() uuid.UUID
	Phone() string
	CreatedAt() time.Time
	UpdatedAt() time.Time
	Messages() []Message
	AppendMessage(msg Message) ChatThread
}

func New(tenantID uuid.UUID, phone string, opts ...Option) ChatThread {
	thread := &chatThread{
		id:        uuid.New(),
		tenantID:  tenantID,
		phone:     phone,
		createdAt: time.Now(),
		updatedAt: time.Now(),
		messages:  nil,
	}

	for _, opt := range opts {
		opt(thread)
	}

	return thread
}

type Option func(*chatThread)

func WithID(id uuid.UUID) Option {
	return func(t *chatThread) {
		if id != uuid.Nil {
			t.id = id
		}
	}
}

func WithCreatedAt(createdAt time.Time) Option {
	return func(t *chatThread) {
		if !createdAt.IsZero() {
			t.createdAt = createdAt
		}
	}
}

func WithUpdatedAt(updatedAt time.Time) Option {
	return func(t *chatThread) {
		if !updatedAt.IsZero() {
			t.updatedAt = updatedAt
		}
	}
}

func WithMessages(messages []Message) Option {
	return func(t *chatThread) {
		t.messages = messages
	}
}

func (t *chatThread) ID() uuid.UUID {
	return t.id
}

func (t *chatThread) TenantID() uuid.UUID {
	return t.tenantID
}

func (t *chatThread) Phone() string {
	return t.phone
}

func (t *chatThread) CreatedAt() time.Time {
	return t.createdAt
}

func (t *chatThread) UpdatedAt() time.Time {
	return t.updatedAt
}

func (t *chatThread) Messages() []Message {
	return t.messages
}

func (t *chatThread) AppendMessage(msg Message) ChatThread {
	if msg == nil {
		return t
	}
	t.messages = append(t.messages, msg)
	if len(t.messages) > MaxMessages {
		t.messages = t.messages[len(t.messages)-MaxMessages:]
	}
	t.updatedAt = msg.Timestamp()
	return t
}
