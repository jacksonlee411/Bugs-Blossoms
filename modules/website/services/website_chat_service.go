package services

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/country"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/phone"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/aichatconfig"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/cache"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/chatthread"
	infraCache "github.com/iota-uz/iota-sdk/modules/website/infrastructure/cache"
	websitePersistence "github.com/iota-uz/iota-sdk/modules/website/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/website/infrastructure/rag"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/intl"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
)

var thinkTagRegex = regexp.MustCompile(`(?s)<think>.*?</think>`)

type CreateThreadDTO struct {
	Phone   string
	Country country.Country
}

type SendMessageToThreadDTO struct {
	ThreadID uuid.UUID
	Message  string
}

type ReplyToThreadDTO struct {
	ThreadID uuid.UUID
	UserID   uint
	Message  string
}

type DefaultWebsiteChatCacheConfig struct {
	Enabled bool
	Prefix  string
	TTL     time.Duration
}

type WebsiteChatServiceConfig struct {
	AIConfigRepo       aichatconfig.Repository
	UserRepo           user.Repository
	ThreadRepo         chatthread.Repository
	AIUserEmail        internet.Email
	RAGProvider        rag.Provider
	DefaultCacheConfig DefaultWebsiteChatCacheConfig
	Cache              cache.Cache
}

type WebsiteChatService struct {
	aiconfigRepo aichatconfig.Repository
	userRepo     user.Repository
	threadRepo   chatthread.Repository
	aiUserEmail  internet.Email
	ragProvider  rag.Provider
	cache        cache.Cache
}

func NewWebsiteChatService(config WebsiteChatServiceConfig) *WebsiteChatService {
	conf := configuration.Use()
	if config.ThreadRepo == nil {
		config.ThreadRepo = websitePersistence.NewInmemThreadRepository()
	}
	service := &WebsiteChatService{
		aiconfigRepo: config.AIConfigRepo,
		userRepo:     config.UserRepo,
		threadRepo:   config.ThreadRepo,
		aiUserEmail:  config.AIUserEmail,
		ragProvider:  config.RAGProvider,
	}

	if config.Cache != nil {
		service.cache = config.Cache
	} else if config.DefaultCacheConfig.Enabled {
		service.cache = infraCache.NewRedisCache(redis.NewClient(&redis.Options{Addr: conf.RedisURL}), config.DefaultCacheConfig.Prefix, config.DefaultCacheConfig.TTL)
	}

	return service
}

func (s *WebsiteChatService) GetThreadByID(ctx context.Context, threadID uuid.UUID) (chatthread.ChatThread, error) {
	return s.threadRepo.GetByID(ctx, threadID)
}

func (s *WebsiteChatService) CreateThread(ctx context.Context, dto CreateThreadDTO) (chatthread.ChatThread, error) {
	var normalizedPhone string
	if strings.TrimSpace(dto.Phone) != "" {
		p, err := phone.Parse(dto.Phone, dto.Country)
		if err != nil {
			return nil, err
		}
		normalizedPhone = p.Value()
	}

	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}
	thread := chatthread.New(tenantID, normalizedPhone)
	return s.threadRepo.Save(ctx, thread)
}

func (s *WebsiteChatService) SendMessageToThread(
	ctx context.Context,
	dto SendMessageToThreadDTO,
) (chatthread.ChatThread, error) {
	thread, err := s.threadRepo.GetByID(ctx, dto.ThreadID)
	if err != nil {
		return nil, err
	}

	msg, err := chatthread.NewMessage(chatthread.RoleUser, dto.Message, time.Now())
	if err != nil {
		return nil, err
	}
	updatedThread := thread.AppendMessage(msg)
	if _, err := s.threadRepo.Save(ctx, updatedThread); err != nil {
		return nil, err
	}

	return updatedThread, nil
}

func (s *WebsiteChatService) ReplyToThread(
	ctx context.Context,
	dto ReplyToThreadDTO,
) (chatthread.ChatThread, error) {
	if _, err := s.userRepo.GetByID(ctx, dto.UserID); err != nil {
		return nil, err
	}

	thread, err := s.threadRepo.GetByID(ctx, dto.ThreadID)
	if err != nil {
		return nil, err
	}
	msg, err := chatthread.NewMessage(chatthread.RoleAssistant, dto.Message, time.Now())
	if err != nil {
		return nil, err
	}

	updatedThread := thread.AppendMessage(msg)
	if _, err := s.threadRepo.Save(ctx, updatedThread); err != nil {
		return nil, err
	}

	return updatedThread, nil
}

func (s *WebsiteChatService) GetAvailableModels(ctx context.Context) ([]string, error) {
	config, err := s.aiconfigRepo.GetDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI configuration: %w", err)
	}

	return s.getModelsWithConfig(ctx, config.BaseURL(), config.AccessToken())
}

func (s *WebsiteChatService) GetAvailableModelsWithConfig(ctx context.Context, baseURL, accessToken string) ([]string, error) {
	if baseURL == "" || accessToken == "" {
		return nil, fmt.Errorf("baseURL and accessToken are required")
	}
	return s.getModelsWithConfig(ctx, baseURL, accessToken)
}

func (s *WebsiteChatService) getModelsWithConfig(ctx context.Context, baseURL, accessToken string) ([]string, error) {
	var openaiClient openai.Client
	if baseURL != "" {
		openaiClient = openai.NewClient(
			option.WithAPIKey(accessToken),
			option.WithBaseURL(baseURL),
		)
	} else {
		openaiClient = openai.NewClient(
			option.WithAPIKey(accessToken),
		)
	}

	response, err := openaiClient.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}

	models := make([]string, 0, len(response.Data))
	for _, model := range response.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

func (s *WebsiteChatService) ReplyWithAI(ctx context.Context, threadID uuid.UUID) (chatthread.ChatThread, error) {
	logger := composables.UseLogger(ctx)
	thread, err := s.GetThreadByID(ctx, threadID)
	if err != nil {
		return nil, err
	}

	messages := thread.Messages()
	if len(messages) == 0 {
		return nil, chatthread.ErrNoMessages
	}

	openaiMessages := []openai.ChatCompletionMessageParamUnion{}

	config, err := s.aiconfigRepo.GetDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI configuration: %w", err)
	}

	if config.SystemPrompt() != "" {
		tmpl, err := template.New("system_prompt").Parse(config.SystemPrompt())
		if err != nil {
			return nil, fmt.Errorf("failed to parse system prompt template: %w", err)
		}

		var buf bytes.Buffer
		templateData := map[string]interface{}{
			"locale": getLocaleString(ctx),
		}
		err = tmpl.Execute(&buf, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to execute system prompt template: %w", err)
		}

		openaiMessages = append(openaiMessages, openai.SystemMessage(buf.String()))
	}

	if s.ragProvider != nil && len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		chunks, err := s.ragProvider.SearchRelevantContext(ctx, lastMessage.Message())
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve context: %w", err)
		}
		if len(chunks) > 0 {
			docsText := strings.Join(chunks, "\n---\n")
			logger.WithFields(logrus.Fields{
				"thread_id":    threadID,
				"chunks":       len(chunks),
				"context":      docsText,
				"user_message": lastMessage.Message(),
			}).Info("Retrieved context for AI response")
			openaiMessages = append(openaiMessages, openai.AssistantMessage("Retrieved context:\n"+docsText))
		}
	}

	for _, msg := range messages {
		if msg.Role() == chatthread.RoleAssistant {
			openaiMessages = append(openaiMessages, openai.AssistantMessage(msg.Message()))
		} else {
			openaiMessages = append(openaiMessages, openai.UserMessage(msg.Message()))
		}
	}
	cachedResponse, err := s.getCachedAIResponse(ctx, config, openaiMessages)
	if err != nil {
		return nil, err
	}
	if cachedResponse != "" {
		logger.WithFields(logrus.Fields{
			"thread_id": threadID,
			"response":  cachedResponse,
		}).Info("Replying with cached response")
		aiUser, err := s.userRepo.GetByEmail(ctx, s.aiUserEmail.Value())
		if err != nil {
			return nil, fmt.Errorf("failed to get AI user: %w", err)
		}
		respThread, err := s.ReplyToThread(ctx, ReplyToThreadDTO{
			ThreadID: threadID,
			UserID:   aiUser.ID(),
			Message:  cachedResponse,
		})

		if err != nil {
			return nil, err
		}

		return respThread, nil
	}

	var openaiClient openai.Client
	if config.BaseURL() != "" {
		openaiClient = openai.NewClient(
			option.WithAPIKey(config.AccessToken()),
			option.WithBaseURL(config.BaseURL()),
		)
	} else {
		openaiClient = openai.NewClient(
			option.WithAPIKey(config.AccessToken()),
		)
	}

	maxTokens := int64(config.MaxTokens())
	temperature := float64(config.Temperature())
	response, err := openaiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       config.ModelName(),
		Messages:    openaiMessages,
		Temperature: openai.Float(temperature),
		MaxTokens:   openai.Int(maxTokens),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get AI response: %w", err)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	rawAIResponse := response.Choices[0].Message.Content

	logger.WithFields(logrus.Fields{
		"thread_id":       threadID,
		"raw_ai_response": rawAIResponse,
	}).Info("Complete AI model output received")

	aiResponse := strings.TrimSpace(thinkTagRegex.ReplaceAllString(rawAIResponse, ""))

	aiUser, err := s.userRepo.GetByEmail(ctx, s.aiUserEmail.Value())
	if err != nil {
		return nil, fmt.Errorf("failed to get AI user: %w", err)
	}

	respThread, err := s.ReplyToThread(ctx, ReplyToThreadDTO{
		ThreadID: threadID,
		UserID:   aiUser.ID(),
		Message:  aiResponse,
	})
	if err != nil {
		return nil, err
	}

	if err := s.saveAIResponse(ctx, config, openaiMessages, aiResponse); err != nil {
		return nil, err
	}

	return respThread, nil
}

func (s *WebsiteChatService) getCacheKey(config aichatconfig.AIConfig, messages []openai.ChatCompletionMessageParamUnion) (string, error) {
	var hashBuffer bytes.Buffer
	configModel := websitePersistence.ToDBConfig(config)
	if err := gob.NewEncoder(&hashBuffer).Encode(configModel); err != nil {
		return "", err
	}
	var messageBuffer bytes.Buffer
	if err := gob.NewEncoder(&messageBuffer).Encode(messages); err != nil {
		return "", err
	}
	if _, err := hashBuffer.Write(messageBuffer.Bytes()); err != nil {
		return "", err
	}
	hash := md5.Sum(hashBuffer.Bytes())
	return hex.EncodeToString(hash[:]), nil
}

func (s *WebsiteChatService) getCachedAIResponse(ctx context.Context, config aichatconfig.AIConfig, messages []openai.ChatCompletionMessageParamUnion) (string, error) {
	if s.cache == nil {
		return "", nil
	}
	key, err := s.getCacheKey(config, messages)
	if err != nil {
		return "", err
	}
	result, err := s.cache.Get(ctx, key)
	if err != nil {
		if errors.Is(err, cache.ErrKeyNotFound) {
			return "", nil
		}
		return "", err
	}
	return result, nil
}

func (s *WebsiteChatService) saveAIResponse(ctx context.Context, config aichatconfig.AIConfig, messages []openai.ChatCompletionMessageParamUnion, response string) error {
	if s.cache == nil {
		return nil
	}
	key, err := s.getCacheKey(config, messages)
	if err != nil {
		return err
	}
	return s.cache.Set(ctx, key, response)
}

func getLocaleString(ctx context.Context) string {
	if locale, ok := intl.UseLocale(ctx); ok {
		return locale.String()
	}
	return language.English.String()
}
