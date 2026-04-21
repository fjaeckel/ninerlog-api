package handlers

import "github.com/prometheus/client_golang/prometheus"

var (
	// AuthLoginAttemptsTotal counts login attempts by result.
	AuthLoginAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_login_attempts_total",
			Help: "Total number of login attempts.",
		},
		[]string{"result"},
	)

	// AuthTokenRefreshTotal counts token refresh attempts by result.
	AuthTokenRefreshTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_token_refresh_total",
			Help: "Total number of token refresh attempts.",
		},
		[]string{"result"},
	)

	// Auth2FAAttemptsTotal counts 2FA verification attempts by result.
	Auth2FAAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_2fa_attempts_total",
			Help: "Total number of 2FA verification attempts.",
		},
		[]string{"result"},
	)
)

func init() {
	prometheus.MustRegister(
		AuthLoginAttemptsTotal,
		AuthTokenRefreshTotal,
		Auth2FAAttemptsTotal,
	)
}
