package service

import (
	"context"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// SetClock replaces the notification service clock.
func (s *NotificationService) SetClock(now func() time.Time) {
	s.now = now
}

// SendLapseNotice sends a lapse notice for refID through the claim flow.
func (s *NotificationService) SendLapseNotice(ctx context.Context, user *models.User, category models.NotificationCategory, refID uuid.UUID, lapsedOn *time.Time) error {
	return s.sendLapse(ctx, user, notice{
		category: category, refID: refID, refType: "test",
		subject: "subject", body: "body", metric: string(category),
	}, lapsedOn)
}

// CrossedThresholds exposes crossedThresholds.
var CrossedThresholds = crossedThresholds
