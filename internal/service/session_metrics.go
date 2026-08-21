package service

import "github.com/prometheus/client_golang/prometheus"

var (
	// SessionsEvictedTotal counts sessions revoked to keep a user within the
	// per-user session cap.
	SessionsEvictedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "auth_sessions_evicted_total",
			Help: "Total number of sessions revoked because the per-user session cap was reached.",
		},
	)

	// RefreshGraceTotal counts refreshes served from a token that had already
	// been rotated but was still inside the reuse grace window.
	RefreshGraceTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "auth_refresh_grace_total",
			Help: "Total number of refreshes served from a superseded token within the reuse grace window.",
		},
	)

	// RefreshReuseDetectedTotal counts refresh tokens presented after their
	// reuse grace elapsed, each of which revoked a session.
	RefreshReuseDetectedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "auth_refresh_reuse_detected_total",
			Help: "Total number of refresh token replays detected after the reuse grace window.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		SessionsEvictedTotal,
		RefreshGraceTotal,
		RefreshReuseDetectedTotal,
	)
}
