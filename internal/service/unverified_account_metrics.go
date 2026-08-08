package service

import "github.com/prometheus/client_golang/prometheus"

var (
	// UnverifiedRemindersTotal counts follow-up verification emails by outcome.
	//
	// Results:
	//   sent          — the reminder was delivered; the deletion clock started.
	//   undeliverable — the address refused mail permanently; the clock started
	//                   anyway, because no later attempt would do better.
	//   deferred      — a transient failure; the account is retried next sweep.
	//   error         — the token could not be minted; nothing was sent.
	UnverifiedRemindersTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unverified_account_reminders_total",
			Help: "Follow-up email-verification reminders by outcome.",
		},
		[]string{"result"},
	)

	// UnverifiedAccountsDeletedTotal counts accounts reaped for never having
	// verified their address. Irreversible, so it is worth alerting on an
	// unexpected jump.
	UnverifiedAccountsDeletedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "unverified_accounts_deleted_total",
			Help: "Accounts deleted after failing to verify their email address within the retention window.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		UnverifiedRemindersTotal,
		UnverifiedAccountsDeletedTotal,
	)
}
