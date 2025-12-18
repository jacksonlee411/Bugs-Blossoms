package subjectid

import (
	"testing"

	"github.com/google/uuid"
)

func TestNormalizedSubjectID(t *testing.T) {
	t.Run("stable mapping with trim", func(t *testing.T) {
		tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
		got, err := NormalizedSubjectID(tenantID, "person", "  000123  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// SSOT: uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s:%s:%s", tenantID, "person", "000123")))
		want := uuid.MustParse("7a5bf0b2-11a4-5b61-ad8e-fbd9ac72ad25")
		if got != want {
			t.Fatalf("unexpected subject_id: got=%s want=%s", got, want)
		}
	})

	t.Run("rejects empty tenant", func(t *testing.T) {
		_, err := NormalizedSubjectID(uuid.Nil, "person", "0001")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("rejects empty pernr after trim", func(t *testing.T) {
		tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
		_, err := NormalizedSubjectID(tenantID, "person", "   ")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("rejects unsupported subject_type", func(t *testing.T) {
		tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
		_, err := NormalizedSubjectID(tenantID, "group", "0001")
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}
