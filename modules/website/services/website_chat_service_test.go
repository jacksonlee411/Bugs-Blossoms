package services_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/country"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/phone"
	corePersistence "github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/chatthread"
	"github.com/iota-uz/iota-sdk/modules/website/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/website/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/itf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChatTest(t *testing.T) (*itf.TestEnvironment, *services.WebsiteChatService, user.Repository) {
	t.Helper()

	fixtures := setupTest(t)

	userRepo := corePersistence.NewUserRepository(corePersistence.NewUploadRepository())
	aiconfigRepo := persistence.NewAIChatConfigRepository()
	threadRepo := persistence.NewInmemThreadRepository()

	websiteChatService := services.NewWebsiteChatService(services.WebsiteChatServiceConfig{
		AIConfigRepo: aiconfigRepo,
		UserRepo:     userRepo,
		ThreadRepo:   threadRepo,
		AIUserEmail:  internet.MustParseEmail("ai@example.com"),
	})

	return fixtures, websiteChatService, userRepo
}

func TestWebsiteChatService_CreateThread_WithPhone(t *testing.T) {
	t.Parallel()
	fixtures, sut, _ := setupChatTest(t)

	phoneStr := "+1-212-664-7667" // Valid US number format
	thread, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   phoneStr,
		Country: country.UnitedStates,
	})
	require.NoError(t, err)
	require.NotNil(t, thread)

	assert.NotEqual(t, uuid.Nil, thread.ID())
	assert.Equal(t, fixtures.TenantID(), thread.TenantID())

	p, err := phone.Parse(phoneStr, country.UnitedStates)
	require.NoError(t, err)
	assert.Equal(t, p.Value(), thread.Phone())

	loadedThread, err := sut.GetThreadByID(fixtures.Ctx, thread.ID())
	require.NoError(t, err)
	assert.Equal(t, thread.ID(), loadedThread.ID())
	assert.Equal(t, thread.Phone(), loadedThread.Phone())
	assert.Equal(t, thread.TenantID(), loadedThread.TenantID())
}

func TestWebsiteChatService_CreateThread_EmptyPhoneAllowed(t *testing.T) {
	t.Parallel()
	fixtures, sut, _ := setupChatTest(t)

	thread, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   "",
		Country: country.UnitedStates,
	})
	require.NoError(t, err)
	require.NotNil(t, thread)

	assert.Empty(t, thread.Phone())
	assert.Equal(t, fixtures.TenantID(), thread.TenantID())
}

func TestWebsiteChatService_CreateThread_InvalidPhone(t *testing.T) {
	t.Parallel()
	fixtures, sut, _ := setupChatTest(t)

	_, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   "not-a-phone-number",
		Country: country.UnitedStates,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, phone.ErrInvalidPhoneNumber)
}

func TestWebsiteChatService_SendMessageToThread(t *testing.T) {
	t.Parallel()
	fixtures, sut, _ := setupChatTest(t)

	thread, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   "+12126647670",
		Country: country.UnitedStates,
	})
	require.NoError(t, err)

	updatedThread, err := sut.SendMessageToThread(fixtures.Ctx, services.SendMessageToThreadDTO{
		ThreadID: thread.ID(),
		Message:  "Hello from client",
	})
	require.NoError(t, err)
	require.NotNil(t, updatedThread)

	messages := updatedThread.Messages()
	require.NotEmpty(t, messages)

	last := messages[len(messages)-1]
	assert.Equal(t, chatthread.RoleUser, last.Role())
	assert.Equal(t, "Hello from client", last.Message())
	assert.False(t, last.Timestamp().IsZero())
}

func TestWebsiteChatService_SendMessageToThread_EmptyMessage(t *testing.T) {
	t.Parallel()
	fixtures, sut, _ := setupChatTest(t)

	thread, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   "+12126647671",
		Country: country.UnitedStates,
	})
	require.NoError(t, err)

	_, err = sut.SendMessageToThread(fixtures.Ctx, services.SendMessageToThreadDTO{
		ThreadID: thread.ID(),
		Message:  "",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatthread.ErrEmptyMessage)
}

func TestWebsiteChatService_SendMessageToThread_MessageTooLong(t *testing.T) {
	t.Parallel()
	fixtures, sut, _ := setupChatTest(t)

	thread, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   "+12126647675",
		Country: country.UnitedStates,
	})
	require.NoError(t, err)

	_, err = sut.SendMessageToThread(fixtures.Ctx, services.SendMessageToThreadDTO{
		ThreadID: thread.ID(),
		Message:  strings.Repeat("a", chatthread.MaxMessageLength+1),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatthread.ErrMessageTooLong)
}

func TestWebsiteChatService_ReplyToThread(t *testing.T) {
	t.Parallel()
	fixtures, sut, userRepo := setupChatTest(t)

	thread, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   "+12126647672",
		Country: country.UnitedStates,
	})
	require.NoError(t, err)

	createdUser := createUserInTx(t, fixtures.Ctx, userRepo, fixtures.TenantID(), user.New(
		"Support",
		"Agent",
		internet.MustParseEmail("test@gmail.com"),
		user.UILanguageEN,
		user.WithTenantID(fixtures.TenantID()),
	))

	repliedThread, err := sut.ReplyToThread(fixtures.Ctx, services.ReplyToThreadDTO{
		ThreadID: thread.ID(),
		UserID:   createdUser.ID(),
		Message:  "Reply from support agent",
	})
	require.NoError(t, err)
	require.NotNil(t, repliedThread)

	messages := repliedThread.Messages()
	require.NotEmpty(t, messages)

	last := messages[len(messages)-1]
	assert.Equal(t, chatthread.RoleAssistant, last.Role())
	assert.Equal(t, "Reply from support agent", last.Message())
	assert.False(t, last.Timestamp().IsZero())
}

func TestWebsiteChatService_ReplyToThread_UserNotFound(t *testing.T) {
	t.Parallel()
	fixtures, sut, _ := setupChatTest(t)

	thread, err := sut.CreateThread(fixtures.Ctx, services.CreateThreadDTO{
		Phone:   "+12126647674",
		Country: country.UnitedStates,
	})
	require.NoError(t, err)

	_, err = sut.ReplyToThread(fixtures.Ctx, services.ReplyToThreadDTO{
		ThreadID: thread.ID(),
		UserID:   999,
		Message:  "This should fail",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, corePersistence.ErrUserNotFound)
}

func createUserInTx(
	t *testing.T,
	ctx context.Context,
	repo user.Repository,
	tenantID uuid.UUID,
	entity user.User,
) user.User {
	t.Helper()

	var created user.User
	err := composables.InTx(ctx, func(txCtx context.Context) error {
		var err error
		created, err = repo.Create(txCtx, entity)
		return err
	})
	require.NoError(t, err)
	return created
}
