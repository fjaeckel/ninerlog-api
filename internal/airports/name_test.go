package airports

import (
	"reflect"
	"testing"
)

func TestFoldName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Konz-Könen Glider Field", "konz konen glider field"},
		{"  Großenhain  ", "grossenhain"},
		{"Grandpa's field", "grandpa s field"},
		{"Saint-Étienne – Bouthéon", "saint etienne boutheon"},
		{"Łódź Lublinek", "lodz lublinek"},
		{"Ærø Flyveplads", "aero flyveplads"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := foldName(tt.in); got != tt.want {
			t.Errorf("foldName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShortNameKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Konz-Könen Glider Field", "konz konen"},
		{"Flugplatz Bienenfarm", ""},
		{"Stade Airport", ""},
		{"Uetersen/Heist Airfield", "uetersen heist"},
		{"Farm", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := shortNameKey(foldName(tt.in)); got != tt.want {
			t.Errorf("shortNameKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidIdent(t *testing.T) {
	for _, id := range []string{"EDDF", "DE-0249", "00AK", "US-1234", "AB-1"} {
		if !validIdent(id) {
			t.Errorf("validIdent(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"", "K00", "02T", "bad ident", "de-0249", "AB-CDEFGHIJ", "EDD F"} {
		if validIdent(id) {
			t.Errorf("validIdent(%q) = true, want false", id)
		}
	}
}

func TestLookupByName(t *testing.T) {
	SetTestDB(map[string]AirportInfo{
		"DE-0249": {Name: "Konz-Könen Glider Field", Latitude: 49.68, Longitude: 6.54},
		"EDHS":    {Name: "Stade Airport", Latitude: 53.56, Longitude: 9.5},
		"US-0001": {Name: "Twin Oaks Airport", Latitude: 40, Longitude: -80},
		"US-0002": {Name: "Twin Oaks Airfield", Latitude: 41, Longitude: -81},
		"XX-0001": {Name: "Bienenfarm", Latitude: 10, Longitude: 10},
		"EPLL":    {Name: "Łódź Władysław Reymont Airport", Latitude: 51.7, Longitude: 19.4},
	})
	defer SetTestDB(nil)

	tests := []struct{ in, want string }{
		{"Lodz Wladyslaw Reymont", "EPLL"},
		{"Bienenfarm", ""},
		{"Konz Könen", "DE-0249"},
		{"konz-konen", "DE-0249"},
		{"Konz-Könen Glider Field", "DE-0249"},
		{"Stade Airport", "EDHS"},
		// Single-word short names, ambiguous names and unknown names do not match
		{"Stade", ""},
		{"Twin Oaks", ""},
		{"Konz", ""},
		{"Meadow strip", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := ""
		if a := LookupByName(tt.in); a != nil {
			got = a.ICAO
		}
		if got != tt.want {
			t.Errorf("LookupByName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLookupByName_NilDB(t *testing.T) {
	SetTestDB(nil)
	if a := LookupByName("Konz Könen"); a != nil {
		t.Errorf("LookupByName with nil db = %v, want nil", a)
	}
}

func TestSearch_ByName(t *testing.T) {
	SetTestDB(map[string]AirportInfo{
		"KONZ":    {Name: "Grosse Ile Municipal Airport", Latitude: 42.1, Longitude: -83.2},
		"DE-0249": {Name: "Konz-Könen Glider Field", Latitude: 49.68, Longitude: 6.54},
		"EDRK":    {Name: "Koblenz-Winningen Airfield", Latitude: 50.33, Longitude: 7.53},
	})
	defer SetTestDB(nil)

	var got []string
	for _, a := range Search("konz", 10) {
		got = append(got, a.ICAO)
	}
	if want := []string{"KONZ", "DE-0249"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Search(konz) = %v, want %v", got, want)
	}

	got = nil
	for _, a := range Search("Könen", 10) {
		got = append(got, a.ICAO)
	}
	if want := []string{"DE-0249"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Search(Könen) = %v, want %v", got, want)
	}

	if r := Search("DE-02", 10); len(r) != 1 || r[0].ICAO != "DE-0249" {
		t.Errorf("Search(DE-02) = %v, want DE-0249", r)
	}
	if r := Search("konz", 1); len(r) != 1 || r[0].ICAO != "KONZ" {
		t.Errorf("Search(konz, 1) = %v, want KONZ only", r)
	}
}
