// Copyright (C) The NinerLog Authors
// SPDX-License-Identifier: AGPL-3.0-only

package models

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateTimeDistribution_CrossCountryBoundedByTotal(t *testing.T) {
	f := &Flight{
		UserID:       uuid.New(),
		Date:         time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "D-EXCC",
		AircraftType: "C172",
		TotalTime:    90,
		IsPIC:        true,
		PICTime:      90,
	}

	f.CrossCountryTime = 90
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Fatalf("cross-country equal to total: unexpected error %v", err)
	}

	f.CrossCountryTime = 91
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidCrossCountryTime) {
		t.Errorf("cross-country above total: got %v, want ErrInvalidCrossCountryTime", err)
	}

	f.CrossCountryTime = -1
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrNegativeTime) {
		t.Errorf("negative cross-country: got %v, want ErrNegativeTime", err)
	}
}
