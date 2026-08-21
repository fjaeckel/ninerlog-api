package middleware

import "github.com/prometheus/client_golang/prometheus"

// AccessTokensRejectedTotal counts authenticated requests refused by the
// session-state check, by reason: session_revoked, account_disabled or
// lookup_failed.
var AccessTokensRejectedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "auth_access_tokens_rejected_total",
		Help: "Total number of requests rejected because the access token's session is no longer usable.",
	},
	[]string{"reason"},
)

func init() {
	prometheus.MustRegister(AccessTokensRejectedTotal)
}
