package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/pkg/hash"
	"github.com/google/uuid"
)

var (
	// ErrSignatureNotFound covers both "no signature with this ID" and, for
	// the public token flow, "no signature with this token" — the latter is
	// aliased to ErrSignatureTokenInvalid so a lookup miss and an
	// already-used token are indistinguishable to an anonymous caller.
	ErrSignatureNotFound = errors.New("signature request not found")

	// ErrSignaturePendingExists is returned when creating a new deferred
	// request while one is already pending for the flight.
	ErrSignaturePendingExists = errors.New("a pending signature request already exists for this flight")

	// ErrSignatureTokenInvalid covers an unknown, revoked, voided, or
	// already-completed token. Deliberately not distinguished from each
	// other (or from "never existed") to avoid leaking state to an
	// unauthenticated caller.
	ErrSignatureTokenInvalid = errors.New("signature link is invalid")
	// ErrSignatureTokenExpired means the token's expiry has passed.
	ErrSignatureTokenExpired = errors.New("signature link has expired")

	// ErrSignatureNotPending is returned by Resend/Revoke when the target
	// signature is not (or no longer) in the 'pending' state.
	ErrSignatureNotPending = errors.New("signature request is not pending")
	// ErrSignatureNotCompleted is returned by Void when the target
	// signature is not in the 'completed' state.
	ErrSignatureNotCompleted = errors.New("signature is not completed")

	ErrSignatureImageTooLarge  = errors.New("signature image exceeds maximum size")
	ErrSignatureImageRequired  = errors.New("a signature image is required")
	ErrSignerNameRequired      = errors.New("signer name is required")
	ErrSignatureReasonRequired = errors.New("a reason is required to void a signature")
)

// FlightSignatureService implements the instructor sign-off workflow: live
// (in-person, synchronous), deferred-by-email, and deferred-by-shareable-link
// (all deferred requests share the same token mechanism; email is just an
// optional delivery channel for it). See models.FlightSignature for the
// lifecycle and internal/service/flight.go's ErrFlightLocked for the
// edit-lock this feature imposes on signed flights.
type FlightSignatureService struct {
	sigRepo    repository.FlightSignatureRepository
	flightRepo repository.FlightRepository
	userRepo   repository.UserRepository
}

func NewFlightSignatureService(sigRepo repository.FlightSignatureRepository, flightRepo repository.FlightRepository, userRepo repository.UserRepository) *FlightSignatureService {
	return &FlightSignatureService{sigRepo: sigRepo, flightRepo: flightRepo, userRepo: userRepo}
}

// getOwnedFlight loads a flight and verifies the caller owns it, translating
// repository errors into the same sentinels FlightService uses.
func (s *FlightSignatureService) getOwnedFlight(ctx context.Context, flightID, userID uuid.UUID) (*models.Flight, error) {
	flight, err := s.flightRepo.GetByID(ctx, flightID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFlightNotFound
		}
		return nil, err
	}
	if flight.UserID != userID {
		return nil, ErrUnauthorizedFlight
	}
	return flight, nil
}

func generateRawToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

func clampExpiryHours(requested *int) int {
	if requested == nil {
		return models.DefaultSignatureRequestExpiryHours
	}
	h := *requested
	if h < models.MinSignatureRequestExpiryHours {
		return models.MinSignatureRequestExpiryHours
	}
	if h > models.MaxSignatureRequestExpiryHours {
		return models.MaxSignatureRequestExpiryHours
	}
	return h
}

// SignLive records an in-person signature captured on the flight owner's own
// device. Completes synchronously; no token is involved.
func (s *FlightSignatureService) SignLive(ctx context.Context, flightID, userID uuid.UUID, signerName string, credentialNo *string, image []byte, signerIP, signerUA string) (*models.FlightSignature, error) {
	flight, err := s.getOwnedFlight(ctx, flightID, userID)
	if err != nil {
		return nil, err
	}
	if flight.SignatureID != nil {
		return nil, ErrFlightLocked
	}
	if strings.TrimSpace(signerName) == "" {
		return nil, ErrSignerNameRequired
	}
	if len(image) == 0 {
		return nil, ErrSignatureImageRequired
	}
	if len(image) > models.MaxSignatureImageBytes {
		return nil, ErrSignatureImageTooLarge
	}

	now := time.Now()
	sig := &models.FlightSignature{
		FlightID:               flightID,
		UserID:                 userID,
		Method:                 models.SignatureMethodLive,
		Status:                 models.SignatureStatusCompleted,
		InstructorName:         &signerName,
		InstructorCredentialNo: credentialNo,
		SignatureImage:         image,
		SignedAt:               &now,
		SignerIP:               nonEmptyPtr(signerIP),
		SignerUserAgent:        nonEmptyPtr(signerUA),
	}
	if err := s.sigRepo.Create(ctx, sig); err != nil {
		return nil, err
	}
	if err := s.flightRepo.SetSignatureLock(ctx, flightID, &sig.ID); err != nil {
		return nil, err
	}
	return sig, nil
}

// CreateRequest starts a deferred (token-based) signature request. If
// instructorEmail is supplied the caller should send the request email
// immediately (see APIHandler.sendSignatureRequestEmail); if omitted, the
// request is created "pending, unsent" so the owner can send it whenever
// they have the instructor's address, or share the link/QR directly.
func (s *FlightSignatureService) CreateRequest(ctx context.Context, flightID, userID uuid.UUID, instructorEmail *string, expiresInHours *int) (sig *models.FlightSignature, rawToken string, err error) {
	flight, err := s.getOwnedFlight(ctx, flightID, userID)
	if err != nil {
		return nil, "", err
	}
	if flight.SignatureID != nil {
		return nil, "", ErrFlightLocked
	}
	if _, err := s.sigRepo.GetPendingByFlightID(ctx, flightID); err == nil {
		return nil, "", ErrSignaturePendingExists
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, "", err
	}

	rawToken, err = generateRawToken()
	if err != nil {
		return nil, "", err
	}
	tokenHash := hash.HashToken(rawToken)
	expiresAt := time.Now().Add(time.Duration(clampExpiryHours(expiresInHours)) * time.Hour)

	sig = &models.FlightSignature{
		FlightID:        flightID,
		UserID:          userID,
		Method:          models.SignatureMethodDeferred,
		Status:          models.SignatureStatusPending,
		TokenHash:       &tokenHash,
		TokenExpiresAt:  &expiresAt,
		InstructorEmail: instructorEmail,
	}
	if err := s.sigRepo.Create(ctx, sig); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, "", ErrSignaturePendingExists
		}
		return nil, "", err
	}
	return sig, rawToken, nil
}

// Resend rotates the token (and, if a new address is supplied, updates the
// delivery email) for a still-pending request. This single action covers
// both "send to a different email" and "regenerate the shareable link/QR" —
// the caller decides whether to also send an email with the result.
func (s *FlightSignatureService) Resend(ctx context.Context, flightID, userID, signatureID uuid.UUID, instructorEmail *string) (sig *models.FlightSignature, rawToken string, err error) {
	sig, err = s.getOwnedPendingSignature(ctx, flightID, userID, signatureID)
	if err != nil {
		return nil, "", err
	}

	rawToken, err = generateRawToken()
	if err != nil {
		return nil, "", err
	}
	tokenHash := hash.HashToken(rawToken)
	expiresAt := time.Now().Add(time.Duration(models.DefaultSignatureRequestExpiryHours) * time.Hour)

	sig.TokenHash = &tokenHash
	sig.TokenExpiresAt = &expiresAt
	if instructorEmail != nil {
		sig.InstructorEmail = instructorEmail
	}
	if err := s.sigRepo.Update(ctx, sig); err != nil {
		return nil, "", err
	}
	return sig, rawToken, nil
}

// MarkEmailSent records that a request email was (re)sent. Call after a
// successful (or best-effort) Sender.Send from the handler layer.
func (s *FlightSignatureService) MarkEmailSent(ctx context.Context, sig *models.FlightSignature) error {
	now := time.Now()
	sig.EmailSentAt = &now
	sig.EmailSendCount++
	return s.sigRepo.Update(ctx, sig)
}

// ExpirePendingRequests soft-expires any pending request whose token has
// passed its expiry, returning the number of rows affected. Called from the
// admin cleanup-tokens maintenance sweep.
func (s *FlightSignatureService) ExpirePendingRequests(ctx context.Context) (int64, error) {
	return s.sigRepo.ExpirePendingPastDue(ctx)
}

// Revoke cancels a still-pending request; nobody has signed it yet.
func (s *FlightSignatureService) Revoke(ctx context.Context, flightID, userID, signatureID uuid.UUID) error {
	sig, err := s.getOwnedPendingSignature(ctx, flightID, userID, signatureID)
	if err != nil {
		return err
	}
	sig.Status = models.SignatureStatusRevoked
	return s.sigRepo.Update(ctx, sig)
}

// Void invalidates an already-completed signature (a reason is required for
// the audit trail) and unlocks the flight for editing again. Editing and
// re-signing after a void is the "revalidation" workflow.
func (s *FlightSignatureService) Void(ctx context.Context, flightID, userID, signatureID uuid.UUID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrSignatureReasonRequired
	}
	sig, err := s.getOwnedSignature(ctx, flightID, userID, signatureID)
	if err != nil {
		return err
	}
	if sig.Status != models.SignatureStatusCompleted {
		return ErrSignatureNotCompleted
	}
	now := time.Now()
	sig.Status = models.SignatureStatusVoided
	sig.VoidedAt = &now
	sig.VoidedReason = &reason
	if err := s.sigRepo.Update(ctx, sig); err != nil {
		return err
	}
	return s.flightRepo.SetSignatureLock(ctx, flightID, nil)
}

// List returns the full signature history for a flight, newest first.
func (s *FlightSignatureService) List(ctx context.Context, flightID, userID uuid.UUID) ([]*models.FlightSignature, error) {
	if _, err := s.getOwnedFlight(ctx, flightID, userID); err != nil {
		return nil, err
	}
	return s.sigRepo.ListByFlightID(ctx, flightID)
}

// Get returns a single signature the caller owns.
func (s *FlightSignatureService) Get(ctx context.Context, flightID, userID, signatureID uuid.UUID) (*models.FlightSignature, error) {
	return s.getOwnedSignature(ctx, flightID, userID, signatureID)
}

func (s *FlightSignatureService) getOwnedSignature(ctx context.Context, flightID, userID, signatureID uuid.UUID) (*models.FlightSignature, error) {
	if _, err := s.getOwnedFlight(ctx, flightID, userID); err != nil {
		return nil, err
	}
	sig, err := s.sigRepo.GetByID(ctx, signatureID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSignatureNotFound
		}
		return nil, err
	}
	if sig.FlightID != flightID {
		return nil, ErrSignatureNotFound
	}
	return sig, nil
}

func (s *FlightSignatureService) getOwnedPendingSignature(ctx context.Context, flightID, userID, signatureID uuid.UUID) (*models.FlightSignature, error) {
	sig, err := s.getOwnedSignature(ctx, flightID, userID, signatureID)
	if err != nil {
		return nil, err
	}
	if sig.Status != models.SignatureStatusPending {
		return nil, ErrSignatureNotPending
	}
	return sig, nil
}

// ResolveToken validates a public signing token and returns the pending
// signature plus its flight, for display on the public /sign/{token} page.
// Never returns information that would let a caller distinguish "token
// never existed" from "token already used/revoked/voided".
func (s *FlightSignatureService) ResolveToken(ctx context.Context, token string) (*models.FlightSignature, *models.Flight, error) {
	sig, err := s.lookupPendingByToken(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	flight, err := s.flightRepo.GetByID(ctx, sig.FlightID)
	if err != nil {
		return nil, nil, err
	}
	return sig, flight, nil
}

// CompleteFromToken records the instructor's signature against a valid
// pending token. Returns the completed signature plus the flight owner's
// DB-sourced email/name so the caller can send a confirmation without ever
// trusting client-supplied contact details (CWE-640).
func (s *FlightSignatureService) CompleteFromToken(ctx context.Context, token, signerName string, credentialNo *string, image []byte, signerIP, signerUA string) (sig *models.FlightSignature, ownerEmail, ownerName string, err error) {
	sig, err = s.lookupPendingByToken(ctx, token)
	if err != nil {
		return nil, "", "", err
	}
	if strings.TrimSpace(signerName) == "" {
		return nil, "", "", ErrSignerNameRequired
	}
	if len(image) == 0 {
		return nil, "", "", ErrSignatureImageRequired
	}
	if len(image) > models.MaxSignatureImageBytes {
		return nil, "", "", ErrSignatureImageTooLarge
	}

	owner, err := s.userRepo.GetByID(ctx, sig.UserID)
	if err != nil {
		return nil, "", "", err
	}

	now := time.Now()
	sig.Status = models.SignatureStatusCompleted
	sig.InstructorName = &signerName
	sig.InstructorCredentialNo = credentialNo
	sig.SignatureImage = image
	sig.SignedAt = &now
	sig.SignerIP = nonEmptyPtr(signerIP)
	sig.SignerUserAgent = nonEmptyPtr(signerUA)
	if err := s.sigRepo.Update(ctx, sig); err != nil {
		return nil, "", "", err
	}
	if err := s.flightRepo.SetSignatureLock(ctx, sig.FlightID, &sig.ID); err != nil {
		return nil, "", "", err
	}
	return sig, owner.Email, owner.Name, nil
}

// lookupPendingByToken hashes the presented token, loads the matching row
// (if any), and enforces the pending/expired invariants shared by
// ResolveToken and CompleteFromToken.
func (s *FlightSignatureService) lookupPendingByToken(ctx context.Context, token string) (*models.FlightSignature, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrSignatureTokenInvalid
	}
	tokenHash := hash.HashToken(token)
	sig, err := s.sigRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSignatureTokenInvalid
		}
		return nil, err
	}

	if sig.Status != models.SignatureStatusPending {
		// completed / revoked / voided / expired all look the same to an
		// anonymous caller — except a lapsed-but-not-yet-swept expiry,
		// handled below, which gets its own distinguishable error since it
		// carries no risk of enumeration (the token was already known).
		if sig.Status == models.SignatureStatusExpired {
			return nil, ErrSignatureTokenExpired
		}
		return nil, ErrSignatureTokenInvalid
	}
	if sig.TokenExpiresAt != nil && sig.TokenExpiresAt.Before(time.Now()) {
		// Lazily flip to 'expired' so the sweep job and this path agree.
		sig.Status = models.SignatureStatusExpired
		_ = s.sigRepo.Update(ctx, sig)
		return nil, ErrSignatureTokenExpired
	}
	return sig, nil
}

func nonEmptyPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
