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

func TestLocalCode(t *testing.T) {
	tests := []struct{ code, ident, want string }{
		{"LF0723", "FR-0009", "LF0723"},
		{" 5m6 ", "0MI1", "5M6"},
		{"A01", "AU-0002", "A01"},
		{"SEE", "KSEE", ""},
		{"EDDF", "EDDF", ""},
		{"AB-12", "XX-0001", ""},
		{"", "XX-0001", ""},
	}
	for _, tt := range tests {
		if got := localCode(tt.code, tt.ident); got != tt.want {
			t.Errorf("localCode(%q, %q) = %q, want %q", tt.code, tt.ident, got, tt.want)
		}
	}
}

func TestLookupCode(t *testing.T) {
	SetTestDB(map[string]AirportInfo{
		"EDDF":    {Name: "Frankfurt Airport", Latitude: 50, Longitude: 8},
		"FR-0009": {Name: "Altisurface Notre-Dame-des-Neiges", Latitude: 45, Longitude: 6, LocalCode: "LF0723"},
		"XX-0001": {Name: "Shadowed", Latitude: 1, Longitude: 1, LocalCode: "EDDF"},
		"US-0001": {Name: "Strip One", Latitude: 40, Longitude: -80, LocalCode: "1N7"},
		"BR-0001": {Name: "Strip Two", Latitude: -10, Longitude: -50, LocalCode: "1N7"},
	})
	defer SetTestDB(nil)

	tests := []struct{ in, want string }{
		{"EDDF", "EDDF"},
		{"lf0723", "FR-0009"},
		{"FR-0009", "FR-0009"},
		{"1N7", ""},
		{"ZZZZ", ""},
	}
	for _, tt := range tests {
		got := ""
		if a := LookupCode(tt.in); a != nil {
			got = a.ICAO
		}
		if got != tt.want {
			t.Errorf("LookupCode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	var codes []string
	for _, a := range Search("LF07", 10) {
		codes = append(codes, a.ICAO)
	}
	if want := []string{"FR-0009"}; !reflect.DeepEqual(codes, want) {
		t.Errorf("Search(LF07) = %v, want %v", codes, want)
	}
	if r := Search("1N", 10); len(r) != 0 {
		t.Errorf("Search(1N) = %v, want none for an ambiguous local code", r)
	}
}

func TestLookupCode_NilDB(t *testing.T) {
	SetTestDB(nil)
	if a := LookupCode("EDDF"); a != nil {
		t.Errorf("LookupCode with nil db = %v, want nil", a)
	}
}
