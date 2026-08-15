package importtemplate

import (
	"sort"
	"testing"
)

// legacyGenericAliases is every header the guess table in import.go resolved
// before the catalogue replaced it. The catalogue is meant to be a superset —
// a header that used to map and now does not is a silent regression for anyone
// importing a hand-kept spreadsheet.
var legacyGenericAliases = []string{
	"date", "flight date", "datum", "aircraft", "registration", "reg", "aircraftid",
	"aircraft reg", "aircraftreg", "tail", "tail number", "a/c reg", "a/c ident",
	"type", "aircraft type", "typecode", "aircrafttype", "a/c type",
	"from", "departure", "dep", "departure icao", "departureicao", "dep place",
	"to", "arrival", "arr", "dest", "arrival icao", "arrivalicao", "arr place",
	"off block", "offblock", "off-block", "out-block", "out block", "timeout",
	"chocks off", "offblocktime", "dep time",
	"on block", "onblock", "on-block", "in-block", "in block", "timein",
	"chocks on", "onblocktime", "arr time",
	"takeoff", "timeoff", "departure time", "departuretime",
	"landing", "timeon", "arrival time", "arrivaltime",
	"total", "total time", "totaltime", "block time", "flight time",
	"pic", "pic time", "dual", "dual received", "dualreceived", "dual rcvd",
	"night", "night time", "nighttime", "ifr", "ifr time", "ifrtime", "instrument",
	"actual instrument", "actualinstrument", "actual inst",
	"simulated instrument", "simulatedinstrument", "sim inst",
	"day landings", "daylandingsfullstop", "day ldg", "landingsday", "ldg day",
	"night landings", "nightlandingsfullstop", "night ldg", "landingsnight", "ldg night",
	"remarks", "comments", "pilotcomments", "notes", "remarks/endorsements",
	"remarks & endorsements", "holds", "approaches", "approachescount",
	"instructorname", "instructor name", "instructorcomments", "instructor comments",
	"dualgiven", "dual given", "instr given",
}

func TestGenericFallbackCoversEveryLegacyAlias(t *testing.T) {
	for _, h := range legacyGenericAliases {
		if len(Suggest(nil, []string{h})) == 0 {
			t.Errorf("header %q mapped under the old guess table but not under the catalogue", h)
		}
	}
}

// Two columns claiming one target field would overwrite each other in the
// importer's randomised map iteration — the same file importing differently on
// different runs. Suggest must never hand the importer that situation, even
// when a file carries several recognised spellings of the same concept.
func TestSuggestNeverClaimsAFieldTwice(t *testing.T) {
	for _, tpl := range All() {
		// Feed every alias the template knows at once: the worst case a real
		// file could ever approximate.
		headers := make([]string, 0, len(tpl.Columns))
		for k := range tpl.Columns {
			headers = append(headers, k)
		}
		sort.Strings(headers)

		claimed := make(map[Field]string)
		for _, m := range tpl.Suggest(headers) {
			if prev, dup := claimed[m.TargetField]; dup {
				t.Errorf("%s: field %s claimed by both %q and %q",
					tpl.ID, m.TargetField, prev, m.SourceColumn)
				continue
			}
			claimed[m.TargetField] = m.SourceColumn
		}
	}
}

// The winner must be the first recognised column in the header row, not
// whichever one the map happened to yield.
func TestSuggestPrefersTheFirstColumnInHeaderOrder(t *testing.T) {
	tpl := ByID("NINERLOG_CSV")

	// NinerLog's own standard export writes both, in this order.
	forward := tpl.Suggest([]string{"Date", "AircraftID", "TotalTime", "Remarks", "Endorsements"})
	reverse := tpl.Suggest([]string{"Date", "AircraftID", "TotalTime", "Endorsements", "Remarks"})

	pick := func(ms []Mapping) string {
		for _, m := range ms {
			if m.TargetField == FieldRemarks {
				return m.SourceColumn
			}
		}
		return ""
	}
	if got := pick(forward); got != "Remarks" {
		t.Errorf("remarks came from %q, want the first column Remarks", got)
	}
	if got := pick(reverse); got != "Endorsements" {
		t.Errorf("remarks came from %q, want the first column Endorsements", got)
	}

	// Repeating the same call must give the same answer every time.
	for i := 0; i < 50; i++ {
		if pick(tpl.Suggest([]string{"Date", "Remarks", "Endorsements"})) != "Remarks" {
			t.Fatal("Suggest is not deterministic across calls")
		}
	}
}
