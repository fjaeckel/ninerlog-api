//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

type reminderBody struct {
	ID                   string  `json:"id"`
	AircraftID           string  `json:"aircraftId"`
	AircraftRegistration string  `json:"aircraftRegistration"`
	Kind                 string  `json:"kind"`
	Label                *string `json:"label"`
	DueDate              string  `json:"dueDate"`
	IntervalMonths       *int    `json:"intervalMonths"`
	LastDoneOn           *string `json:"lastDoneOn"`
	Notes                *string `json:"notes"`
	Status               string  `json:"status"`
	DaysUntilDue         int     `json:"daysUntilDue"`
}

func createReminderAircraft(t *testing.T, c *E2EClient, reg string) string {
	t.Helper()
	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": reg, "type": "C42", "make": "Comco Ikarus", "model": "C42",
		"aircraftClass": "ULTRALIGHT", "ulKind": "THREE_AXIS",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	return ac["id"].(string)
}

func createReminder(t *testing.T, c *E2EClient, aircraftID string, body map[string]interface{}) reminderBody {
	t.Helper()
	resp := c.POST("/aircraft/"+aircraftID+"/reminders", body)
	requireStatus(t, resp, http.StatusCreated)
	var r reminderBody
	if err := resp.JSON(&r); err != nil {
		t.Fatalf("decode reminder: %v", err)
	}
	return r
}

func listReminders(t *testing.T, c *E2EClient, path string) []reminderBody {
	t.Helper()
	resp := c.GET(path)
	requireStatus(t, resp, http.StatusOK)
	var out []reminderBody
	if err := resp.JSON(&out); err != nil {
		t.Fatalf("decode reminders: %v", err)
	}
	return out
}

func TestAircraftReminders_CRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("reminders"), "SecurePass123!", "Mehmet")
	c42 := createReminderAircraft(t, c, "D-MXYZ")
	base := "/aircraft/" + c42 + "/reminders"

	var annual reminderBody

	t.Run("M job 3: Jahresnachprüfung with a 12-month interval", func(t *testing.T) {
		annual = createReminder(t, c, c42, map[string]interface{}{
			"kind": "ANNUAL_INSPECTION", "dueDate": futureDate(10), "intervalMonths": 12,
		})
		if annual.AircraftRegistration != "D-MXYZ" || annual.AircraftID != c42 {
			t.Errorf("unexpected aircraft on reminder: %+v", annual)
		}
		if annual.Status != "due_soon" || annual.DaysUntilDue != 10 {
			t.Errorf("status = %s/%d, want due_soon/10", annual.Status, annual.DaysUntilDue)
		}
		if annual.IntervalMonths == nil || *annual.IntervalMonths != 12 {
			t.Errorf("intervalMonths = %v, want 12", annual.IntervalMonths)
		}
	})

	t.Run("M job 3: rescue-system repack and rocket expiry", func(t *testing.T) {
		createReminder(t, c, c42, map[string]interface{}{"kind": "RESCUE_SYSTEM_REPACK", "dueDate": futureDate(200), "intervalMonths": 72})
		rocket := createReminder(t, c, c42, map[string]interface{}{"kind": "RESCUE_ROCKET_EXPIRY", "dueDate": pastDate(2)})
		if rocket.Status != "overdue" || rocket.DaysUntilDue != -2 {
			t.Errorf("status = %s/%d, want overdue/-2", rocket.Status, rocket.DaysUntilDue)
		}
	})

	t.Run("validation", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]interface{}
		}{
			{"custom without label", map[string]interface{}{"kind": "CUSTOM", "dueDate": futureDate(5)}},
			{"custom with blank label", map[string]interface{}{"kind": "CUSTOM", "label": "  ", "dueDate": futureDate(5)}},
			{"unknown kind", map[string]interface{}{"kind": "OIL_CHANGE", "dueDate": futureDate(5)}},
			{"interval zero", map[string]interface{}{"kind": "ARC", "dueDate": futureDate(5), "intervalMonths": 0}},
			{"interval 241", map[string]interface{}{"kind": "ARC", "dueDate": futureDate(5), "intervalMonths": 241}},
			{"label too long", map[string]interface{}{"kind": "CUSTOM", "label": strings.Repeat("x", 101), "dueDate": futureDate(5)}},
			{"notes too long", map[string]interface{}{"kind": "ARC", "notes": strings.Repeat("x", 1001), "dueDate": futureDate(5)}},
			{"missing due date", map[string]interface{}{"kind": "ARC"}},
			{"malformed due date", map[string]interface{}{"kind": "ARC", "dueDate": "next week"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertStatus(t, c.POST(base, tc.body), http.StatusBadRequest)
			})
		}
		t.Run("custom with label and 240-month interval is accepted", func(t *testing.T) {
			r := createReminder(t, c, c42, map[string]interface{}{
				"kind": "CUSTOM", "label": "Engine TBO", "dueDate": futureDate(900), "intervalMonths": 240,
			})
			if r.Label == nil || *r.Label != "Engine TBO" || r.Status != "ok" {
				t.Errorf("unexpected custom reminder: %+v", r)
			}
		})
	})

	t.Run("list is ordered by due date", func(t *testing.T) {
		list := listReminders(t, c, base)
		if len(list) != 4 {
			t.Fatalf("got %d reminders, want 4", len(list))
		}
		for i := 1; i < len(list); i++ {
			if list[i].DueDate < list[i-1].DueDate {
				t.Errorf("not ordered by due date: %s before %s", list[i-1].DueDate, list[i].DueDate)
			}
		}
		if list[0].Kind != "RESCUE_ROCKET_EXPIRY" {
			t.Errorf("first = %s, want the overdue rocket", list[0].Kind)
		}
	})

	t.Run("patch updates and null clears", func(t *testing.T) {
		resp := c.PATCH(base+"/"+annual.ID, map[string]interface{}{"notes": "DULV Prüfer booked"})
		requireStatus(t, resp, http.StatusOK)
		var r reminderBody
		resp.JSON(&r)
		if r.Notes == nil || *r.Notes != "DULV Prüfer booked" || r.IntervalMonths == nil {
			t.Errorf("unexpected reminder after patch: %+v", r)
		}
		resp = c.PATCH(base+"/"+annual.ID, map[string]interface{}{"notes": nil})
		requireStatus(t, resp, http.StatusOK)
		var cleared reminderBody
		resp.JSON(&cleared)
		if cleared.Notes != nil {
			t.Errorf("notes = %v, want cleared", *cleared.Notes)
		}
		if cleared.IntervalMonths == nil || *cleared.IntervalMonths != 12 {
			t.Errorf("clearing notes touched intervalMonths: %v", cleared.IntervalMonths)
		}
		assertStatus(t, c.PATCH(base+"/"+annual.ID, map[string]interface{}{"kind": "CUSTOM"}), http.StatusBadRequest)
		assertStatus(t, c.PATCH(base+"/"+annual.ID, map[string]interface{}{"intervalMonths": 0}), http.StatusBadRequest)
	})

	t.Run("M job 3: completing the Jahresnachprüfung rolls the due date 12 months", func(t *testing.T) {
		resp := c.POST(base+"/"+annual.ID+"/complete", map[string]interface{}{"doneOn": "2026-10-02"})
		requireStatus(t, resp, http.StatusOK)
		var r reminderBody
		resp.JSON(&r)
		if r.DueDate != "2027-10-02" || r.LastDoneOn == nil || *r.LastDoneOn != "2026-10-02" {
			t.Errorf("after complete: due=%s lastDone=%v, want 2027-10-02/2026-10-02", r.DueDate, r.LastDoneOn)
		}
	})

	t.Run("completing on Jan 31 with a 1-month interval clamps to Feb 28", func(t *testing.T) {
		r := createReminder(t, c, c42, map[string]interface{}{"kind": "ELT_BATTERY", "dueDate": "2026-01-31", "intervalMonths": 1})
		resp := c.POST(base+"/"+r.ID+"/complete", map[string]interface{}{"doneOn": "2026-01-31"})
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&r)
		if r.DueDate != "2026-02-28" {
			t.Errorf("dueDate = %s, want 2026-02-28", r.DueDate)
		}
		requireStatus(t, c.DELETE(base+"/"+r.ID), http.StatusNoContent)
	})

	t.Run("completing without body defaults to today", func(t *testing.T) {
		r := createReminder(t, c, c42, map[string]interface{}{"kind": "INSURANCE", "dueDate": futureDate(3), "intervalMonths": 12})
		resp := c.POST(base+"/"+r.ID+"/complete", nil)
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&r)
		todayUTC := time.Now().UTC().Format("2006-01-02")
		if r.LastDoneOn == nil || *r.LastDoneOn != todayUTC {
			t.Errorf("lastDoneOn = %v, want %s", r.LastDoneOn, todayUTC)
		}
		if r.Status != "ok" {
			t.Errorf("status after completion = %s, want ok", r.Status)
		}
		requireStatus(t, c.DELETE(base+"/"+r.ID), http.StatusNoContent)
	})

	t.Run("completing without interval leaves the due date", func(t *testing.T) {
		r := createReminder(t, c, c42, map[string]interface{}{"kind": "ARC", "dueDate": "2026-12-01"})
		resp := c.POST(base+"/"+r.ID+"/complete", map[string]interface{}{"doneOn": "2026-11-20"})
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&r)
		if r.DueDate != "2026-12-01" || r.LastDoneOn == nil || *r.LastDoneOn != "2026-11-20" {
			t.Errorf("after complete: due=%s lastDone=%v", r.DueDate, r.LastDoneOn)
		}
		requireStatus(t, c.DELETE(base+"/"+r.ID), http.StatusNoContent)
	})

	t.Run("delete", func(t *testing.T) {
		r := createReminder(t, c, c42, map[string]interface{}{"kind": "ARC", "dueDate": futureDate(100)})
		requireStatus(t, c.DELETE(base+"/"+r.ID), http.StatusNoContent)
		assertStatus(t, c.DELETE(base+"/"+r.ID), http.StatusNotFound)
		assertStatus(t, c.PATCH(base+"/"+r.ID, map[string]interface{}{"notes": "x"}), http.StatusNotFound)
	})

	t.Run("unknown aircraft is 404", func(t *testing.T) {
		assertStatus(t, c.GET("/aircraft/00000000-0000-0000-0000-000000000000/reminders"), http.StatusNotFound)
	})

	t.Run("requires auth", func(t *testing.T) {
		anon := NewE2EClient(t)
		assertStatus(t, anon.GET(base), http.StatusUnauthorized)
		assertStatus(t, anon.GET("/aircraft-reminders"), http.StatusUnauthorized)
	})
}

func TestAircraftReminders_CrossUserIsNotFound(t *testing.T) {
	owner := NewE2EClient(t)
	registerAndLogin(t, owner, uniqueEmail("rem-owner"), "SecurePass123!", "Mehmet")
	c42 := createReminderAircraft(t, owner, "D-MXYZ")
	rem := createReminder(t, owner, c42, map[string]interface{}{"kind": "INSURANCE", "dueDate": futureDate(20)})

	intruder := NewE2EClient(t)
	registerAndLogin(t, intruder, uniqueEmail("rem-intruder"), "SecurePass123!", "Intruder")
	own := createReminderAircraft(t, intruder, "D-EFGH")

	base := "/aircraft/" + c42 + "/reminders"
	for name, resp := range map[string]*Response{
		"list":                      intruder.GET(base),
		"create":                    intruder.POST(base, map[string]interface{}{"kind": "ARC", "dueDate": futureDate(5)}),
		"patch":                     intruder.PATCH(base+"/"+rem.ID, map[string]interface{}{"notes": "mine now"}),
		"complete":                  intruder.POST(base+"/"+rem.ID+"/complete", nil),
		"delete":                    intruder.DELETE(base + "/" + rem.ID),
		"patch via own aircraft":    intruder.PATCH("/aircraft/"+own+"/reminders/"+rem.ID, map[string]interface{}{"notes": "x"}),
		"delete via own aircraft":   intruder.DELETE("/aircraft/" + own + "/reminders/" + rem.ID),
		"complete via own aircraft": intruder.POST("/aircraft/"+own+"/reminders/"+rem.ID+"/complete", nil),
	} {
		t.Run(name, func(t *testing.T) { assertStatus(t, resp, http.StatusNotFound) })
	}

	t.Run("dashboard list excludes other users", func(t *testing.T) {
		if n := len(listReminders(t, intruder, "/aircraft-reminders")); n != 0 {
			t.Errorf("intruder sees %d reminders, want 0", n)
		}
	})

	t.Run("owner's reminder is untouched", func(t *testing.T) {
		list := listReminders(t, owner, base)
		if len(list) != 1 || list[0].Notes != nil {
			t.Fatalf("owner's reminders changed: %+v", list)
		}
	})
}

func TestAircraftReminders_DashboardAcrossAircraft(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("rem-dash"), "SecurePass123!", "Mehmet")
	c42 := createReminderAircraft(t, c, "D-MXYZ")
	trike := createReminderAircraft(t, c, "D-MTRK")

	createReminder(t, c, c42, map[string]interface{}{"kind": "ANNUAL_INSPECTION", "dueDate": futureDate(25)})
	createReminder(t, c, trike, map[string]interface{}{"kind": "INSURANCE", "dueDate": futureDate(5)})
	createReminder(t, c, trike, map[string]interface{}{"kind": "RESCUE_ROCKET_EXPIRY", "dueDate": pastDate(1)})
	createReminder(t, c, c42, map[string]interface{}{"kind": "ELT_BATTERY", "dueDate": futureDate(400)})

	t.Run("all reminders ordered by due date with registration", func(t *testing.T) {
		list := listReminders(t, c, "/aircraft-reminders")
		if len(list) != 4 {
			t.Fatalf("got %d, want 4", len(list))
		}
		want := []string{"D-MTRK", "D-MTRK", "D-MXYZ", "D-MXYZ"}
		for i, r := range list {
			if r.AircraftRegistration != want[i] {
				t.Errorf("[%d] registration = %s, want %s", i, r.AircraftRegistration, want[i])
			}
		}
	})

	t.Run("dueWithinDays=30 keeps due-soon and overdue", func(t *testing.T) {
		list := listReminders(t, c, "/aircraft-reminders?dueWithinDays=30")
		if len(list) != 3 {
			t.Fatalf("got %d, want 3", len(list))
		}
		if list[0].Status != "overdue" || list[1].Status != "due_soon" {
			t.Errorf("statuses = %s,%s want overdue,due_soon", list[0].Status, list[1].Status)
		}
	})

	t.Run("dueWithinDays=0 keeps overdue only", func(t *testing.T) {
		if n := len(listReminders(t, c, "/aircraft-reminders?dueWithinDays=0")); n != 1 {
			t.Errorf("got %d, want 1", n)
		}
	})

	t.Run("invalid dueWithinDays is 400", func(t *testing.T) {
		assertStatus(t, c.GET("/aircraft-reminders?dueWithinDays=-1"), http.StatusBadRequest)
		assertStatus(t, c.GET("/aircraft-reminders?dueWithinDays=soon"), http.StatusBadRequest)
	})

	t.Run("deleting an aircraft deletes its reminders", func(t *testing.T) {
		requireStatus(t, c.DELETE("/aircraft/"+trike), http.StatusNoContent)
		list := listReminders(t, c, "/aircraft-reminders")
		if len(list) != 2 {
			t.Fatalf("got %d reminders after aircraft delete, want 2", len(list))
		}
		for _, r := range list {
			if r.AircraftRegistration == "D-MTRK" {
				t.Error("reminder survived its aircraft")
			}
		}
		assertStatus(t, c.GET("/aircraft/"+trike+"/reminders"), http.StatusNotFound)
	})
}

func TestAircraftReminders_AdminStats(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("rem-admin"), "SecurePass123!", "Mehmet")
	c42 := createReminderAircraft(t, c, "D-MXYZ")
	createReminder(t, c, c42, map[string]interface{}{"kind": "RESCUE_ROCKET_EXPIRY", "dueDate": pastDate(3)})

	resp := ac.GET("/admin/stats")
	requireStatus(t, resp, http.StatusOK)
	var s struct {
		AircraftReminders *struct {
			Total   *int `json:"total"`
			Overdue *int `json:"overdue"`
		} `json:"aircraftReminders"`
	}
	if err := resp.JSON(&s); err != nil {
		t.Fatal(err)
	}
	if s.AircraftReminders == nil || s.AircraftReminders.Total == nil || s.AircraftReminders.Overdue == nil {
		t.Fatalf("admin stats missing aircraftReminders.{total,overdue}: %s", string(resp.Body))
	}
	if *s.AircraftReminders.Total < 1 || *s.AircraftReminders.Overdue < 1 {
		t.Errorf("aircraftReminders = %d/%d, want at least 1/1", *s.AircraftReminders.Total, *s.AircraftReminders.Overdue)
	}
	if *s.AircraftReminders.Overdue > *s.AircraftReminders.Total {
		t.Error("overdue cannot exceed total")
	}
}

func TestAircraftReminders_NotificationCategoryDefaultsOn(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("rem-prefs"), "SecurePass123!", "Mehmet")
	resp := c.GET("/users/me/notifications")
	requireStatus(t, resp, http.StatusOK)
	var prefs struct {
		EnabledCategories []string `json:"enabledCategories"`
	}
	resp.JSON(&prefs)
	for _, cat := range prefs.EnabledCategories {
		if cat == "aircraft_reminder" {
			return
		}
	}
	t.Errorf("aircraft_reminder not enabled by default: %v", prefs.EnabledCategories)
}

func TestAircraftReminders_Notification(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("rem-notify")
	registerAndLogin(t, c, email, "SecurePass123!", "Mehmet")
	requireStatus(t, c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"aircraft_reminder"},
		"warningDays":       []int{30},
	}), http.StatusOK)

	c42 := createReminderAircraft(t, c, "D-MXYZ")
	createReminder(t, c, c42, map[string]interface{}{"kind": "ANNUAL_INSPECTION", "dueDate": futureDate(10), "intervalMonths": 12})

	t.Run("M job 3: Jahresnachprüfung due soon notifies", func(t *testing.T) {
		requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
		var msg *MailPitFullMessage
		for i := 0; i < 20 && msg == nil; i++ {
			time.Sleep(500 * time.Millisecond)
			msg = mailpitFindEmail(t, email, "D-MXYZ")
		}
		if msg == nil {
			t.Fatalf("no aircraft reminder email to %s", email)
		}
		if !strings.Contains(msg.Subject, "Annual inspection") || !strings.Contains(msg.Subject, "due in 10 days") {
			t.Errorf("subject = %q", msg.Subject)
		}
		if !strings.Contains(msg.HTML, "Aircraft Reminder") || !strings.Contains(msg.HTML, "Mehmet") {
			t.Errorf("unexpected body: %s", msg.HTML)
		}
	})

	t.Run("a second check does not resend the same threshold", func(t *testing.T) {
		requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
		time.Sleep(2 * time.Second)
		result := mailpitSearchByRecipient(t, email)
		n := 0
		for _, m := range result.Messages {
			if strings.Contains(m.Subject, "D-MXYZ") {
				n++
			}
		}
		if n != 1 {
			t.Errorf("got %d reminder emails, want 1", n)
		}
	})

	t.Run("history lists the aircraft_reminder category", func(t *testing.T) {
		resp := c.GET("/users/me/notifications/history")
		requireStatus(t, resp, http.StatusOK)
		if !strings.Contains(string(resp.Body), `"aircraft_reminder"`) {
			t.Errorf("history missing aircraft_reminder: %s", string(resp.Body))
		}
	})
}
