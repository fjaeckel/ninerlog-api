package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/errcode"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validBackup = `{
  "format": "NinerLog JSON Backup",
  "version": "1.0",
  "flights": [{"date": "2026-01-02T00:00:00Z", "aircraftReg": "D-EABC", "aircraftType": "C172", "totalTime": 60, "picTime": 60, "isPic": true,
               "crewMembers": [{"name": "Sam Carter", "role": "Instructor"}]}],
  "aircraft": [{"registration": "D-EABC", "type": "C172", "make": "Cessna", "model": "172S"}],
  "licenses": [],
  "credentials": [],
  "contacts": [{"name": "Sam Carter"}],
  "customCurrencyRules": [{"name": "Stale", "enabled": true, "notify": true,
      "definition": {"window": {"amount": 90, "unit": "fortnights"}, "requirements": [{"metric": "landings", "min": 3}]}}],
  "customReports": []
}`

// The deprecated and the consolidated route run the same code; every case
// below runs against both.
var restoreRoutes = map[string]func(h *APIHandler, userID uuid.UUID, mode string, body []byte) *httptest.ResponseRecorder{
	"POST /users/me/data": func(h *APIHandler, userID uuid.UUID, mode string, body []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c := authenticatedContext(w, userID)
		c.Request = httptest.NewRequest("POST", "/users/me/data", bytes.NewReader(body))
		var params generated.RestoreUserDataParams
		if mode != "" {
			m := generated.RestoreUserDataParamsMode(mode)
			params.Mode = &m
		}
		h.RestoreUserData(c, params)
		return w
	},
	"POST /imports/json": func(h *APIHandler, userID uuid.UUID, mode string, body []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c := authenticatedContext(w, userID)
		c.Request = httptest.NewRequest("POST", "/imports/json", bytes.NewReader(body))
		var params generated.ImportDataJSONParams
		if mode != "" {
			m := generated.ImportDataJSONParamsMode(mode)
			params.Mode = &m
		}
		h.ImportDataJSON(c, params)
		return w
	},
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	return out
}

func TestRestoreUserData_Routes(t *testing.T) {
	for route, call := range restoreRoutes {
		t.Run(route, func(t *testing.T) {
			t.Run("restores into an empty account", func(t *testing.T) {
				h, store, userID := restoreHandler(t)

				w := call(h, userID, "", []byte(validBackup))

				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				body := decode(t, w)
				assert.Equal(t, "restore", body["mode"])
				assert.EqualValues(t, 1, body["flightsImported"])
				assert.EqualValues(t, 1, body["aircraftImported"])
				assert.EqualValues(t, 1, body["crewMembersImported"])
				assert.EqualValues(t, 1, body["customCurrencyRulesImported"])
				warnings, ok := body["warnings"].([]any)
				require.True(t, ok)
				require.Len(t, warnings, 1)
				assert.Equal(t, "customCurrencyRules", warnings[0].(map[string]any)["section"])
				assert.Len(t, store.State.Flights, 1)
				assert.False(t, store.State.Rules[0].Enabled)
			})

			t.Run("a second restore is refused and writes nothing", func(t *testing.T) {
				h, store, userID := restoreHandler(t)
				require.Equal(t, http.StatusOK, call(h, userID, "", []byte(validBackup)).Code)

				w := call(h, userID, "restore", []byte(validBackup))

				require.Equal(t, http.StatusConflict, w.Code)
				assert.Equal(t, errcode.AccountNotEmpty, decode(t, w)["code"])
				assert.Len(t, store.State.Flights, 1)
			})

			t.Run("replace yields one copy", func(t *testing.T) {
				h, store, userID := restoreHandler(t)
				require.Equal(t, http.StatusOK, call(h, userID, "", []byte(validBackup)).Code)

				w := call(h, userID, "replace", []byte(validBackup))

				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				assert.Equal(t, "replace", decode(t, w)["mode"])
				assert.Len(t, store.State.Flights, 1)
				assert.Len(t, store.State.Aircraft, 1)
				assert.Len(t, store.State.Contacts, 1)
			})

			t.Run("replace is refused while attachments exist", func(t *testing.T) {
				h, store, userID := restoreHandler(t)
				store.Attachments = repository.RestoreAttachments{SignedFlights: true}
				store.State.Flights = []*models.Flight{{AircraftReg: "D-EOLD"}}

				w := call(h, userID, "replace", []byte(validBackup))

				require.Equal(t, http.StatusConflict, w.Code)
				assert.Equal(t, errcode.ReplaceWouldDropAttachments, decode(t, w)["code"])
				assert.Equal(t, "D-EOLD", store.State.Flights[0].AircraftReg)
			})

			t.Run("rejects an unknown mode", func(t *testing.T) {
				h, store, userID := restoreHandler(t)

				w := call(h, userID, "merge", []byte(validBackup))

				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, 0, store.Commits)
			})

			t.Run("rejects malformed JSON without echoing the parser error", func(t *testing.T) {
				h, _, userID := restoreHandler(t)

				w := call(h, userID, "", []byte(`{"format": "NinerLog JSON Backup", "flights": [`))

				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "Invalid JSON body", decode(t, w)["error"])
			})

			t.Run("rejects another format", func(t *testing.T) {
				h, _, userID := restoreHandler(t)

				w := call(h, userID, "", []byte(`{"format": "Something Else"}`))

				assert.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("refuses a newer major version", func(t *testing.T) {
				h, store, userID := restoreHandler(t)

				w := call(h, userID, "", []byte(`{"format": "NinerLog JSON Backup", "version": "2.0"}`))

				require.Equal(t, http.StatusUnprocessableEntity, w.Code)
				assert.Equal(t, errcode.BackupVersionUnsupported, decode(t, w)["code"])
				assert.Equal(t, 0, store.Commits)
			})

			t.Run("accepts a backup without a version", func(t *testing.T) {
				h, _, userID := restoreHandler(t)

				w := call(h, userID, "", []byte(`{"format": "NinerLog JSON Backup"}`))

				assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			})

			t.Run("answers 413 when the body exceeds the cap", func(t *testing.T) {
				h, store, userID := restoreHandler(t)
				w := httptest.NewRecorder()
				c := authenticatedContext(w, userID)
				c.Request = httptest.NewRequest("POST", "/users/me/data", strings.NewReader(validBackup))
				c.Request.Body = http.MaxBytesReader(w, c.Request.Body, 64)

				h.RestoreUserData(c, generated.RestoreUserDataParams{})

				assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
				assert.Equal(t, 0, store.Commits)
			})

			t.Run("hides internal errors and leaves the account unchanged", func(t *testing.T) {
				h, store, userID := restoreHandler(t)
				store.FailAt = func(op string, n int) error {
					if op == restorefake.OpFlights {
						return errors.New(`pq: duplicate key value violates unique constraint "flights_pkey"`)
					}
					return nil
				}

				w := call(h, userID, "", []byte(validBackup))

				require.Equal(t, http.StatusInternalServerError, w.Code)
				assert.NotContains(t, w.Body.String(), "pq:")
				assert.NotContains(t, w.Body.String(), "flights_pkey")
				assert.Empty(t, store.State.Aircraft)
			})

			t.Run("requires a session", func(t *testing.T) {
				h, _, _ := restoreHandler(t)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("POST", "/users/me/data", strings.NewReader(validBackup))

				if route == "POST /users/me/data" {
					h.RestoreUserData(c, generated.RestoreUserDataParams{})
				} else {
					h.ImportDataJSON(c, generated.ImportDataJSONParams{})
				}

				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})
		})
	}
}

type fakeContentRepo struct {
	deletedAll     []uuid.UUID
	deletedFlights []uuid.UUID
	flightCount    int64
	err            error
}

func (r *fakeContentRepo) DeleteAllContent(ctx context.Context, userID uuid.UUID) error {
	r.deletedAll = append(r.deletedAll, userID)
	return r.err
}

func (r *fakeContentRepo) DeleteFlightsAndHistory(ctx context.Context, userID uuid.UUID) (int64, error) {
	r.deletedFlights = append(r.deletedFlights, userID)
	return r.flightCount, r.err
}

var _ repository.UserContentRepository = (*fakeContentRepo)(nil)

func deleteHandler(repo *fakeContentRepo) *APIHandler {
	h, _ := setupTestHandler()
	h.userDataService = service.NewUserDataService(repo)
	return h
}

func TestDeleteAllUserData_Scopes(t *testing.T) {
	scope := func(s generated.DeleteAllUserDataParamsScope) generated.DeleteAllUserDataParams {
		return generated.DeleteAllUserDataParams{Scope: &s}
	}

	t.Run("default scope wipes everything", func(t *testing.T) {
		repo := &fakeContentRepo{}
		userID := uuid.New()
		w := httptest.NewRecorder()
		c := authenticatedContext(w, userID)
		c.Request = httptest.NewRequest("DELETE", "/users/me/data", nil)

		deleteHandler(repo).DeleteAllUserData(c, generated.DeleteAllUserDataParams{})

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, []uuid.UUID{userID}, repo.deletedAll)
		assert.Empty(t, repo.deletedFlights)
		body := decode(t, w)
		assert.Equal(t, "All user data deleted successfully", body["message"])
		assert.NotContains(t, body, "deletedFlights")
	})

	t.Run("scope=all wipes everything", func(t *testing.T) {
		repo := &fakeContentRepo{}
		w := httptest.NewRecorder()
		c := authenticatedContext(w, uuid.New())
		c.Request = httptest.NewRequest("DELETE", "/users/me/data?scope=all", nil)

		deleteHandler(repo).DeleteAllUserData(c, scope(generated.DeleteAllUserDataParamsScopeAll))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, repo.deletedAll, 1)
	})

	t.Run("scope=flights deletes flights and reports the count", func(t *testing.T) {
		repo := &fakeContentRepo{flightCount: 42}
		userID := uuid.New()
		w := httptest.NewRecorder()
		c := authenticatedContext(w, userID)
		c.Request = httptest.NewRequest("DELETE", "/users/me/data?scope=flights", nil)

		deleteHandler(repo).DeleteAllUserData(c, scope(generated.DeleteAllUserDataParamsScopeFlights))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, []uuid.UUID{userID}, repo.deletedFlights)
		assert.Empty(t, repo.deletedAll)
		body := decode(t, w)
		assert.EqualValues(t, 42, body["deletedFlights"])
		assert.NotEmpty(t, body["message"])
	})

	t.Run("unknown scope is a 400 and deletes nothing", func(t *testing.T) {
		repo := &fakeContentRepo{}
		w := httptest.NewRecorder()
		c := authenticatedContext(w, uuid.New())
		c.Request = httptest.NewRequest("DELETE", "/users/me/data?scope=users", nil)

		deleteHandler(repo).DeleteAllUserData(c, scope("users"))

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, repo.deletedAll)
		assert.Empty(t, repo.deletedFlights)
	})

	t.Run("failures are generic", func(t *testing.T) {
		for _, s := range []generated.DeleteAllUserDataParamsScope{
			generated.DeleteAllUserDataParamsScopeAll, generated.DeleteAllUserDataParamsScopeFlights,
		} {
			repo := &fakeContentRepo{err: errors.New("pq: deadlock detected")}
			w := httptest.NewRecorder()
			c := authenticatedContext(w, uuid.New())
			c.Request = httptest.NewRequest("DELETE", "/users/me/data", nil)

			deleteHandler(repo).DeleteAllUserData(c, scope(s))

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.NotContains(t, w.Body.String(), "deadlock")
		}
	})
}

func TestDeleteAllFlights_KeepsItsResponseShape(t *testing.T) {
	repo := &fakeContentRepo{flightCount: 7}
	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("DELETE", "/flights/delete-all", nil)

	deleteHandler(repo).DeleteAllFlights(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, map[string]any{"deleted": float64(7)}, decode(t, w))
	assert.Equal(t, []uuid.UUID{userID}, repo.deletedFlights)
	assert.Empty(t, repo.deletedAll)
}

func TestDeleteAllFlights_FailureIsReportedNotIgnored(t *testing.T) {
	repo := &fakeContentRepo{err: errors.New("pq: could not delete flight_imports")}
	w := httptest.NewRecorder()
	c := authenticatedContext(w, uuid.New())
	c.Request = httptest.NewRequest("DELETE", "/flights/delete-all", nil)

	deleteHandler(repo).DeleteAllFlights(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "flight_imports")
}

func TestExportUserData_RequiresASession(t *testing.T) {
	h, _ := setupTestHandler()
	for name, call := range map[string]func(c *gin.Context){
		"GET /users/me/data": h.ExportUserData,
		"GET /exports/json":  h.ExportDataJSON,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/users/me/data", nil)

			call(c)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	read, write time.Time
}

func (d *deadlineRecorder) SetReadDeadline(t time.Time) error  { d.read = t; return nil }
func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error { d.write = t; return nil }

func TestExtendDeadlines_LiftsServerReadAndWriteDeadlines(t *testing.T) {
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)

	extendDeadlines(c)

	for name, got := range map[string]time.Time{"read": w.read, "write": w.write} {
		if left := time.Until(got); left < 4*time.Minute || left > UserDataRequestBudget {
			t.Errorf("%s deadline is %v away, want about %v", name, left, UserDataRequestBudget)
		}
	}
}
