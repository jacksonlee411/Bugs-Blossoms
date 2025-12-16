package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/chatthread"
	"github.com/iota-uz/iota-sdk/modules/website/infrastructure/persistence/models"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/redis/go-redis/v9"
)

type ThreadRepository struct {
	redis  *redis.Client
	prefix string
}

func NewThreadRepository(redis *redis.Client) *ThreadRepository {
	return &ThreadRepository{redis: redis, prefix: "website:ai_chat:threads:v2"}
}

func (r *ThreadRepository) GetByID(ctx context.Context, id uuid.UUID) (chatthread.ChatThread, error) {
	var model models.ChatThread
	hashKey, err := r.hashKey(ctx)
	if err != nil {
		return nil, err
	}
	result, err := r.redis.HGet(ctx, hashKey, id.String()).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, chatthread.ErrChatThreadNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(result), &model); err != nil {
		return nil, err
	}

	return ToDomainChatThread(model)
}

func (r *ThreadRepository) Save(ctx context.Context, thread chatthread.ChatThread) (chatthread.ChatThread, error) {
	hashKey, err := r.hashKey(ctx)
	if err != nil {
		return nil, err
	}
	threadJson, err := json.Marshal(ToDBChatThread(thread))
	if err != nil {
		return nil, err
	}
	if err := r.redis.HSet(ctx, hashKey, thread.ID().String(), threadJson).Err(); err != nil {
		return nil, err
	}

	return thread, nil
}

func (r *ThreadRepository) Delete(ctx context.Context, id uuid.UUID) error {
	hashKey, err := r.hashKey(ctx)
	if err != nil {
		return err
	}
	return r.redis.HDel(ctx, hashKey, id.String()).Err()
}

func (r *ThreadRepository) List(ctx context.Context) ([]chatthread.ChatThread, error) {
	hashKey, err := r.hashKey(ctx)
	if err != nil {
		return nil, err
	}
	resultMap, err := r.redis.HGetAll(ctx, hashKey).Result()
	if err != nil {
		return nil, err
	}
	threads := make([]chatthread.ChatThread, 0, len(resultMap))
	for _, value := range resultMap {
		var model models.ChatThread
		if err := json.Unmarshal([]byte(value), &model); err != nil {
			return nil, err
		}
		thread, err := ToDomainChatThread(model)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}

	return threads, nil
}

func (r *ThreadRepository) hashKey(ctx context.Context) (string, error) {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:{%s}", r.prefix, tenantID.String()), nil
}
