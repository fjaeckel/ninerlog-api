package service

import "github.com/prometheus/client_golang/prometheus"

var (
	// WebAuthnSessionsCreatedTotal counts ceremony sessions opened, split by
	// ceremony ("registration" | "login").
	WebAuthnSessionsCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webauthn_sessions_created_total",
			Help: "Total number of WebAuthn ceremony sessions opened.",
		},
		[]string{"ceremony"},
	)

	// WebAuthnSessionsConsumedTotal counts finish attempts by ceremony and
	// result ("ok" | "rejected"). A rejection covers every unusable handle:
	// expired, already consumed, wrong ceremony, or forged.
	WebAuthnSessionsConsumedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webauthn_sessions_consumed_total",
			Help: "Total number of WebAuthn ceremony finish attempts by result.",
		},
		[]string{"ceremony", "result"},
	)

	// WebAuthnSessionsEvictedTotal counts sessions dropped by the per-user
	// open ceremony cap.
	WebAuthnSessionsEvictedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "webauthn_sessions_evicted_total",
			Help: "Total number of WebAuthn sessions evicted by the per-user open ceremony cap.",
		},
	)

	// WebAuthnSessionsExpiredTotal counts rows removed by the cleanup reaper.
	WebAuthnSessionsExpiredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "webauthn_sessions_expired_total",
			Help: "Total number of expired WebAuthn sessions removed by the cleanup job.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		WebAuthnSessionsCreatedTotal,
		WebAuthnSessionsConsumedTotal,
		WebAuthnSessionsEvictedTotal,
		WebAuthnSessionsExpiredTotal,
	)
}
