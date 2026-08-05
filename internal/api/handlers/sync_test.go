package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type stubDeletionRepo struct {
	tombstones []*models.Deletion
}

func (s *stubDeletionRepo) matching(userID uuid.UUID, since time.Time, entity *models.DeletionEntityType) []*models.Deletion {
	var out []*models.Deletion
	for _, d := range s.tombstones {
		if d.UserID != userID || !d.DeletedAt.After(since) {
			continue
		}
		if entity != nil && d.EntityType != *entity {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (s *stubDeletionRepo) ListSince(_ context.Context, userID uuid.UUID, since time.Time,
	entity *models.DeletionEntityType, limit, offset int,
) ([]*models.Deletion, error) {
	all := s.matching(userID, since, entity)
	if offset >= len(all) {
		return nil, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (s *stubDeletionRepo) CountSince(_ context.Context, userID uuid.UUID, since time.Time,
	entity *models.DeletionEntityType,
) (int, error) {
	return len(s.matching(userID, since, entity)), nil
}

func (s *stubDeletionRepo) DeleteExpired(context.Context, time.Time) (int64, error) { return 0, nil }

type deletionFeedBody struct {
	Data       []map[string]interface{} `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		Total      int `json:"total"`
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
	RetentionDays    int  `json:"retentionDays"`
	WatermarkExpired bool `json:"watermarkExpired"`
}

func syncHandler(tombstones ...*models.Deletion) *APIHandler {
	gin.SetMode(gin.TestMode)
	h := &APIHandler{}
	h.SetDeletionService(service.NewDeletionService(
		&stubDeletionRepo{tombstones: tombstones}, service.DefaultTombstoneRetention,
	))
	return h
}

func TestListDeletionsHandler(t *testing.T) {
	userID := uuid.New()
	now := time.Now()
	watermark := now.Add(-time.Hour)

	t.Run("returns the feed envelope", func(t *testing.T) {
		flightID := uuid.New()
		h := syncHandler(&models.Deletion{
			UserID: userID, EntityType: models.DeletionEntityFlight,
			EntityID: flightID, DeletedAt: now.Add(-30 * time.Minute),
		})

		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		h.ListDeletions(c, generated.ListDeletionsParams{Since: watermark})

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		var body deletionFeedBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Data) != 1 {
			t.Fatalf("returned %d deletions, want 1", len(body.Data))
		}
		if body.Data[0]["entity"] != "flight" || body.Data[0]["id"] != flightID.String() {
			t.Errorf("deletion = %v, want the flight %s", body.Data[0], flightID)
		}
		if body.Pagination.Total != 1 || body.Pagination.TotalPages != 1 {
			t.Errorf("pagination = %+v, want total 1 over 1 page", body.Pagination)
		}
		if body.RetentionDays != 90 {
			t.Errorf("retentionDays = %d, want 90", body.RetentionDays)
		}
		if body.WatermarkExpired {
			t.Error("a fresh watermark must not be flagged expired")
		}
	})

	t.Run("an empty feed serialises as a list, not null", func(t *testing.T) {
		h := syncHandler()
		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		h.ListDeletions(c, generated.ListDeletionsParams{Since: watermark})

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if string(raw["data"]) != "[]" {
			t.Errorf("data = %s, want []", raw["data"])
		}
	})

	t.Run("flags an expired watermark", func(t *testing.T) {
		h := syncHandler()
		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		h.ListDeletions(c, generated.ListDeletionsParams{
			Since: now.Add(-service.DefaultTombstoneRetention - time.Hour),
		})

		var body deletionFeedBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !body.WatermarkExpired {
			t.Error("a watermark older than retention must set watermarkExpired")
		}
	})

	t.Run("echoes the clamped page size", func(t *testing.T) {
		h := syncHandler()
		oversized := 10_000
		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		h.ListDeletions(c, generated.ListDeletionsParams{Since: watermark, PageSize: &oversized})

		var body deletionFeedBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Pagination.PageSize != service.MaxDeletionPageSize {
			t.Errorf("pageSize = %d, want the %d cap rather than the requested %d",
				body.Pagination.PageSize, service.MaxDeletionPageSize, oversized)
		}
	})

	t.Run("scopes to the caller", func(t *testing.T) {
		h := syncHandler(&models.Deletion{
			UserID: uuid.New(), EntityType: models.DeletionEntityFlight,
			EntityID: uuid.New(), DeletedAt: now.Add(-30 * time.Minute),
		})

		w := httptest.NewRecorder()
		c := deltaSyncContext(w, userID)
		h.ListDeletions(c, generated.ListDeletionsParams{Since: watermark})

		var body deletionFeedBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Data) != 0 {
			t.Errorf("returned %d of another user's deletions, want 0", len(body.Data))
		}
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		h := syncHandler()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		h.ListDeletions(c, generated.ListDeletionsParams{Since: watermark})

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})
}
