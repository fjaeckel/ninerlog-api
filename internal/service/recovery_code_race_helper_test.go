package service_test

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// raceUserRepo models the atomic semantics the SQL provides: removal and the
// "did I remove it" answer happen as one indivisible step. A mutex here stands
// in for the single conditional UPDATE; a mock that instead did
// read-then-write would reproduce the original bug.
type raceUserRepo struct {
	mu     sync.Mutex
	userID uuid.UUID
	stored []string
}

func newRaceUserRepo(codes ...string) *raceUserRepo {
	return &raceUserRepo{userID: uuid.New(), stored: append([]string{}, codes...)}
}

func (r *raceUserRepo) ConsumeRecoveryCode(_ context.Context, id uuid.UUID, codeHash string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id != r.userID {
		return false, nil
	}
	for i, h := range r.stored {
		if h == codeHash {
			r.stored = append(r.stored[:i], r.stored[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (r *raceUserRepo) codes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.stored...)
}
