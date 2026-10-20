package postgres

import (
	"database/sql"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
)

// txRepos binds the repositories a write hook reads and writes to one
// transaction.
type txRepos struct {
	tx *sql.Tx
}

func (t *txRepos) Contacts() repository.ContactRepository { return &ContactRepository{tx: t.tx} }

func (t *txRepos) Logbook() repository.LogbookRepository { return &logbookRepository{tx: t.tx} }

func (t *txRepos) Licenses() repository.LicenseRepository { return &licenseRepository{tx: t.tx} }
