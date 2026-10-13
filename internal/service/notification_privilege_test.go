package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type countingNotifUserRepo struct {
	*mockUserRepoForSignature
	getByID int
}

func (m *countingNotifUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	m.getByID++
	return m.mockUserRepoForSignature.GetByID(ctx, id)
}

func TestCheckPrivilegeExpiry_OutsideWindowSkipsUserLookup(t *testing.T) {
	prefs := &models.NotificationPreferences{
		UserID:            uuid.New(),
		WarningDays:       pq.Int64Array{30, 14, 7},
		EnabledCategories: pq.StringArray{string(models.NotifCategoryRatingExpiry)},
	}
	for name, expiry := range map[string]time.Time{
		"far future": time.Now().AddDate(1, 0, 0),
		"expired":    time.Now().AddDate(0, 0, -3),
	} {
		t.Run(name, func(t *testing.T) {
			users := &countingNotifUserRepo{mockUserRepoForSignature: newMockUserRepoForSignature()}
			s := &NotificationService{userRepo: users}
			s.checkPrivilegeExpiry(context.Background(), prefs,
				currency.PrivilegeCurrency{PrivilegeID: uuid.New(), Kind: "towing", ExpiresOn: &expiry}, "a@b.c", "A")
			if users.getByID != 0 {
				t.Errorf("user loaded %d times, want 0", users.getByID)
			}
		})
	}
}
