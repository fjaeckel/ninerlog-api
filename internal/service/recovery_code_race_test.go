package service_test

import (
	"context"
	"sync"
	"testing"
)

// Recovery codes are single-use. Consumption used to be a read-modify-write on
// the user row with no lock or version check: load the user, drop the matching
// entry in Go, write the whole row back. Concurrent submissions of the SAME
// code all observed it as present, all matched, and all authenticated --
// confirmed against a running instance, where ten parallel /auth/2fa/login
// requests with one code returned 200 ten times.
//
// Consumption now goes through an atomic conditional UPDATE
// (array_remove ... WHERE $hash = ANY(recovery_codes)), so exactly one caller
// wins. This test drives the same race through the repository contract.
func TestConsumeRecoveryCode_ExactlyOneWinnerUnderConcurrency(t *testing.T) {
	const attempts = 20
	repo := newRaceUserRepo("hash-A", "hash-B")

	var wg sync.WaitGroup
	results := make([]bool, attempts)
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // release all goroutines together
			ok, err := repo.ConsumeRecoveryCode(context.Background(), repo.userID, "hash-A")
			if err != nil {
				t.Errorf("attempt %d: %v", i, err)
			}
			results[i] = ok
		}(i)
	}
	close(start)
	wg.Wait()

	won := 0
	for _, ok := range results {
		if ok {
			won++
		}
	}
	if won != 1 {
		t.Errorf("a single-use recovery code was consumed %d times, want exactly 1", won)
	}

	// The unrelated code must survive, and the used one must be gone.
	remaining := repo.codes()
	if len(remaining) != 1 || remaining[0] != "hash-B" {
		t.Errorf("remaining codes = %v, want [hash-B]", remaining)
	}
}

func TestConsumeRecoveryCode_UnknownHashDoesNotConsume(t *testing.T) {
	repo := newRaceUserRepo("hash-A")
	ok, err := repo.ConsumeRecoveryCode(context.Background(), repo.userID, "not-a-stored-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("an unknown hash reported as consumed")
	}
	if len(repo.codes()) != 1 {
		t.Error("stored codes were modified by a non-matching hash")
	}
}
