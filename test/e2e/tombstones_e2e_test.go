//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// GET /sync/deletions closes the gap updatedSince leaves open: a deleted record
// simply stops appearing in the list endpoints, so a client mirroring the
// logbook would keep it forever. Together the two make a full sync pass — pull
// changes, then pull deletions — expressible without reading the whole logbook.

type deletionFeed struct {
	Data []struct {
		Entity    string `json:"entity"`
		ID        string `json:"id"`
		DeletedAt string `json:"deletedAt"`
	} `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		Total      int `json:"total"`
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
	RetentionDays    int  `json:"retentionDays"`
	WatermarkExpired bool `json:"watermarkExpired"`
}

func deletionsSince(t *testing.T, c *E2EClient, since string, extra ...string) deletionFeed {
	t.Helper()
	path := "/sync/deletions?since=" + url.QueryEscape(since)
	for _, e := range extra {
		path += "&" + e
	}
	r := c.GET(path)
	requireStatus(t, r, http.StatusOK)
	var feed deletionFeed
	if err := r.JSON(&feed); err != nil {
		t.Fatalf("decode deletion feed: %v", err)
	}
	return feed
}

// ids returns the deleted ids for one entity type.
func (f deletionFeed) ids(entity string) []string {
	var out []string
	for _, d := range f.Data {
		if d.Entity == entity {
			out = append(out, d.ID)
		}
	}
	return out
}

func TestTombstonesReportDeletions(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tomb"), "SecurePass123!", "Tomb User")

	// A watermark from before anything exists: the account was created moments
	// ago, so this is "everything that ever happened to this logbook".
	epoch := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	if feed := deletionsSince(t, c, epoch); feed.Pagination.Total != 0 {
		t.Fatalf("a fresh account reported %d deletions, want 0", feed.Pagination.Total)
	}

	// Create one of each synced entity, then delete it.
	requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Doomed Contact"}), http.StatusCreated)
	contacts := listRecords(t, c, "/contacts")
	contactID := idOf(t, contacts[0])

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-ETMB", "type": "C172", "make": "Cessna", "model": "172S",
		"aircraftClass": "SEP_LAND",
	}), http.StatusCreated)
	aircraft, _ := listPaged(t, c, "/aircraft")
	aircraftID := idOf(t, aircraft[0])

	requireStatus(t, c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "DE-TMB-1",
		"issueDate": "2023-01-15", "issuingAuthority": "LBA",
	}), http.StatusCreated)
	licenses := listRecords(t, c, "/licenses")
	licenseID := idOf(t, licenses[0])

	requireStatus(t, c.POST("/credentials", map[string]interface{}{
		"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": "2024-01-15",
		"expiryDate": futureDate(365), "issuingAuthority": "AME Smith",
	}), http.StatusCreated)
	credentials := listRecords(t, c, "/credentials")
	credentialID := idOf(t, credentials[0])

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-ETMB", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
	}), http.StatusCreated)
	flights, _ := listPaged(t, c, "/flights")
	flightID := idOf(t, flights[0])

	// Creating things must not produce deletions.
	if feed := deletionsSince(t, c, epoch); feed.Pagination.Total != 0 {
		t.Fatalf("creating records reported %d deletions, want 0", feed.Pagination.Total)
	}

	assertStatus(t, c.DELETE("/flights/"+flightID), http.StatusNoContent)
	assertStatus(t, c.DELETE("/contacts/"+contactID), http.StatusNoContent)
	assertStatus(t, c.DELETE("/aircraft/"+aircraftID), http.StatusNoContent)
	assertStatus(t, c.DELETE("/credentials/"+credentialID), http.StatusNoContent)
	assertStatus(t, c.DELETE("/licenses/"+licenseID), http.StatusNoContent)

	feed := deletionsSince(t, c, epoch)
	if feed.Pagination.Total != 5 {
		t.Fatalf("reported %d deletions, want 5: %+v", feed.Pagination.Total, feed.Data)
	}
	for entity, want := range map[string]string{
		"flight": flightID, "contact": contactID, "aircraft": aircraftID,
		"credential": credentialID, "license": licenseID,
	} {
		got := feed.ids(entity)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s deletions = %v, want [%s]", entity, got, want)
		}
	}

	t.Run("oldest first", func(t *testing.T) {
		for i := 1; i < len(feed.Data); i++ {
			if feed.Data[i].DeletedAt < feed.Data[i-1].DeletedAt {
				t.Fatalf("feed is not oldest-first: %s before %s",
					feed.Data[i-1].DeletedAt, feed.Data[i].DeletedAt)
			}
		}
	})

	t.Run("reports the retention window", func(t *testing.T) {
		if feed.RetentionDays <= 0 {
			t.Errorf("retentionDays = %d, want a positive window", feed.RetentionDays)
		}
		if feed.WatermarkExpired {
			t.Error("a watermark from an hour ago must not be flagged expired")
		}
	})

	t.Run("a watermark after the deletions is empty", func(t *testing.T) {
		future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
		if got := deletionsSince(t, c, future); got.Pagination.Total != 0 {
			t.Errorf("a future watermark reported %d deletions, want 0", got.Pagination.Total)
		}
	})

	t.Run("advancing the watermark past a deletion drops it", func(t *testing.T) {
		// Take the watermark from the feed itself, as a client would.
		last := feed.Data[len(feed.Data)-1].DeletedAt
		if got := deletionsSince(t, c, last); got.Pagination.Total != 0 {
			t.Errorf("replaying the last deletedAt reported %d deletions, want 0 — strictly-after", got.Pagination.Total)
		}
	})

	t.Run("filters by entity", func(t *testing.T) {
		got := deletionsSince(t, c, epoch, "entity=flight")
		if got.Pagination.Total != 1 {
			t.Fatalf("entity=flight reported %d deletions, want 1", got.Pagination.Total)
		}
		if got.Data[0].ID != flightID {
			t.Errorf("entity=flight returned %s, want %s", got.Data[0].ID, flightID)
		}
	})

	t.Run("pages", func(t *testing.T) {
		page1 := deletionsSince(t, c, epoch, "page=1", "pageSize=2")
		if page1.Pagination.Total != 5 || page1.Pagination.TotalPages != 3 {
			t.Fatalf("pagination = %+v, want 5 items over 3 pages", page1.Pagination)
		}
		if len(page1.Data) != 2 {
			t.Fatalf("page 1 held %d deletions, want 2", len(page1.Data))
		}
		seen := map[string]bool{}
		for page := 1; page <= 3; page++ {
			p := deletionsSince(t, c, epoch, fmt.Sprintf("page=%d", page), "pageSize=2")
			for _, d := range p.Data {
				if seen[d.ID] {
					t.Errorf("id %s appeared on two pages", d.ID)
				}
				seen[d.ID] = true
			}
		}
		if len(seen) != 5 {
			t.Errorf("paging surfaced %d of 5 deletions", len(seen))
		}
	})
}

// The bulk paths delete without going through a per-record endpoint. They are
// exactly the case a Go-side implementation would have missed.
func TestTombstonesCoverBulkDeletes(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tomb-bulk"), "SecurePass123!", "Tomb Bulk")
	epoch := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	for _, reg := range []string{"D-EBLK1", "D-EBLK2", "D-EBLK3"} {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": reg, "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		}), http.StatusCreated)
	}

	requireStatus(t, c.DELETE("/flights/delete-all"), http.StatusOK)

	feed := deletionsSince(t, c, epoch, "entity=flight")
	if feed.Pagination.Total != 3 {
		t.Fatalf("delete-all reported %d deletions, want 3", feed.Pagination.Total)
	}
}

// An account-data wipe must be reported too: the user keeps their login, so a
// client is still syncing and needs to drop everything it mirrors.
func TestTombstonesCoverUserDataWipe(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tomb-wipe"), "SecurePass123!", "Tomb Wipe")
	epoch := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Wiped Contact"}), http.StatusCreated)
	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EWIP", "type": "C172", "make": "Cessna", "model": "172S",
		"aircraftClass": "SEP_LAND",
	}), http.StatusCreated)
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-EWIP", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
	}), http.StatusCreated)

	resp := c.DELETE("/users/me/data")
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("wipe returned %d: %s", resp.StatusCode, resp.Body)
	}

	feed := deletionsSince(t, c, epoch)
	if feed.Pagination.Total < 3 {
		t.Errorf("the wipe reported %d deletions, want at least the 3 records created: %+v",
			feed.Pagination.Total, feed.Data)
	}
}

func TestDeletionFeedValidation(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tomb-bad"), "SecurePass123!", "Tomb Bad")
	epoch := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	t.Run("since is required", func(t *testing.T) {
		// Without a watermark the feed has no meaning; answering with
		// everything would be a silent, unbounded response.
		assertStatus(t, c.GET("/sync/deletions"), http.StatusBadRequest)
	})

	t.Run("a malformed since is rejected", func(t *testing.T) {
		for _, bad := range []string{"yesterday", "1754388525", "2026-08-05 10:08:45"} {
			assertStatus(t, c.GET("/sync/deletions?since="+url.QueryEscape(bad)), http.StatusBadRequest)
		}
	})

	t.Run("an unknown entity is rejected", func(t *testing.T) {
		// Answering with an empty feed would read as "nothing was deleted",
		// which is the one answer a sync client must never get wrong.
		assertStatus(t, c.GET("/sync/deletions?since="+url.QueryEscape(epoch)+"&entity=spaceship"),
			http.StatusBadRequest)
	})

	t.Run("requires authentication", func(t *testing.T) {
		anon := NewE2EClient(t)
		assertStatus(t, anon.GET("/sync/deletions?since="+url.QueryEscape(epoch)), http.StatusUnauthorized)
	})
}

// One pilot must never see another's deletions, even though a tombstone carries
// only an id and a type.
func TestDeletionFeedIsUserScoped(t *testing.T) {
	owner := NewE2EClient(t)
	registerAndLogin(t, owner, uniqueEmail("tomb-owner"), "SecurePass123!", "Tomb Owner")
	epoch := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	requireStatus(t, owner.POST("/contacts", map[string]interface{}{"name": "Owner Contact"}), http.StatusCreated)
	contacts := listRecords(t, owner, "/contacts")
	assertStatus(t, owner.DELETE("/contacts/"+idOf(t, contacts[0])), http.StatusNoContent)

	if got := deletionsSince(t, owner, epoch); got.Pagination.Total != 1 {
		t.Fatalf("the owner saw %d deletions, want 1", got.Pagination.Total)
	}

	intruder := NewE2EClient(t)
	registerAndLogin(t, intruder, uniqueEmail("tomb-intruder"), "SecurePass123!", "Tomb Intruder")
	if got := deletionsSince(t, intruder, epoch); got.Pagination.Total != 0 {
		t.Errorf("another user saw %d of the owner's deletions, want 0", got.Pagination.Total)
	}
}
