package authz

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestSubjectForUser(t *testing.T) {
	t.Run("global tenant defaults", func(t *testing.T) {
		userID := uuid.MustParse("f6f8b13e-755f-41e0-af1a-f2671e40c15c")
		subject := SubjectForUser(uuid.Nil, userID)
		assert.Equal(t, "tenant:global:user:"+userID.String(), subject)
	})

	t.Run("anonymous fallback", func(t *testing.T) {
		subject := SubjectForUserID(uuid.MustParse("274b29c7-86cb-4da1-85a3-3a221fe62a72"), "")
		assert.Contains(t, subject, "anonymous")
	})
}

func TestObjectName(t *testing.T) {
	assert.Equal(t, "core.users", ObjectName("CORE", "Users"))
	assert.Equal(t, "global.resource", ObjectName("", ""))
}

func TestNormalizeAction(t *testing.T) {
	assert.Equal(t, "edit", NormalizeAction(" Edit "))
	assert.Equal(t, "*", NormalizeAction(""))
}
