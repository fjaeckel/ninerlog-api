package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestStatsByClass_SplitsUltralightByKind(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	cols := []string{"class", "ul_kind", "flights", "minutes", "pic_minutes", "dual_minutes", "landings"}
	mock.ExpectQuery("GROUP BY 1, 2").WillReturnRows(sqlmock.NewRows(cols).
		AddRow("SEP_LAND", nil, 4, 240, 240, 0, 4).
		AddRow("ULTRALIGHT", "THREE_AXIS", 10, 600, 600, 0, 20).
		AddRow("ULTRALIGHT", "WEIGHT_SHIFT", 3, 90, 60, 30, 3).
		AddRow("ULTRALIGHT", nil, 1, 20, 20, 0, 1).
		AddRow("GLIDER", nil, 6, 48, 48, 0, 6))

	repo := &reportsRepository{db: db}
	rows, err := repo.StatsByClass(context.Background(), uuid.New(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d class rows, want 3", len(rows))
	}

	t.Run("M rows ordered by minutes with the ultralight row summed", func(t *testing.T) {
		ul := rows[0]
		if ul.Class != "ULTRALIGHT" || ul.Flights != 14 || ul.Minutes != 710 || ul.PICMinutes != 680 || ul.DualMinutes != 30 || ul.Landings != 24 {
			t.Errorf("ultralight row = %+v", *ul)
		}
		if rows[1].Class != "SEP_LAND" || rows[2].Class != "GLIDER" {
			t.Errorf("class order = %s, %s", rows[1].Class, rows[2].Class)
		}
	})

	t.Run("S1 kinds apart, kindless flights on their own", func(t *testing.T) {
		kinds := rows[0].ULKinds
		if len(kinds) != 3 {
			t.Fatalf("got %d kinds, want 3", len(kinds))
		}
		want := []struct {
			kind    string
			minutes int
		}{{"THREE_AXIS", 600}, {"WEIGHT_SHIFT", 90}, {"", 20}}
		for i, w := range want {
			got := ""
			if kinds[i].ULKind != nil {
				got = *kinds[i].ULKind
			}
			if got != w.kind || kinds[i].Minutes != w.minutes {
				t.Errorf("kind %d = %q/%d, want %q/%d", i, got, kinds[i].Minutes, w.kind, w.minutes)
			}
		}
	})

	t.Run("A1 other classes carry no kind split", func(t *testing.T) {
		for _, r := range rows[1:] {
			if r.ULKinds != nil {
				t.Errorf("%s has a UL kind split", r.Class)
			}
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
