package flightsearch

import (
	"strings"
	"testing"
	"time"
)

// compile is a test helper: parse + compile starting at $1.
func compile(t *testing.T, input string) (string, []interface{}) {
	t.Helper()
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", input, err)
	}
	sql, args := q.Compile(1)
	return sql, args
}

func TestFreeTextTerm(t *testing.T) {
	sql, args := compile(t, "EDDF")
	if len(args) != 1 || args[0] != "%EDDF%" {
		t.Fatalf("expected single %%EDDF%% arg, got %v", args)
	}
	for _, col := range []string{"aircraft_reg", "departure_icao", "route", "remarks", "pic_name"} {
		if !strings.Contains(sql, col) {
			t.Errorf("free text SQL missing column %s: %s", col, sql)
		}
	}
	if !strings.Contains(sql, "flight_crew_members") {
		t.Errorf("free text SQL should search crew names: %s", sql)
	}
}

func TestQuotedFreeTextTerm(t *testing.T) {
	_, args := compile(t, `"John Doe"`)
	if len(args) != 1 || args[0] != "%John Doe%" {
		t.Fatalf("expected quoted phrase arg, got %v", args)
	}
}

func TestTagContains(t *testing.T) {
	sql, args := compile(t, "departure:EDDF")
	if !strings.Contains(sql, "COALESCE(departure_icao, '') ILIKE $1") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
	if args[0] != "%EDDF%" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestTagExactAndNotEqual(t *testing.T) {
	sql, args := compile(t, "aircraftReg=D-EFGH")
	if !strings.Contains(sql, "LOWER(COALESCE(aircraft_reg, '')) = LOWER($1)") || args[0] != "D-EFGH" {
		t.Fatalf("unexpected: %s %v", sql, args)
	}
	sql, _ = compile(t, "aircraftReg!=D-EFGH")
	if !strings.Contains(sql, "<> LOWER($1)") {
		t.Fatalf("unexpected: %s", sql)
	}
}

func TestAliases(t *testing.T) {
	for _, q := range []string{"reg:D-E", "from:EDDF", "to:EDDH", "xc>0", "night>0", "instructor:smith", "ifr>=30"} {
		if _, err := Parse(q); err != nil {
			t.Errorf("alias query %q should parse: %v", q, err)
		}
	}
}

func TestDurationFormats(t *testing.T) {
	cases := map[string]int{
		"totalTime>90":   90,
		"totalTime>1:30": 90,
		"totalTime>1.5h": 90,
		"totalTime>2h":   120,
	}
	for input, want := range cases {
		_, args := compile(t, input)
		if len(args) != 1 || args[0] != want {
			t.Errorf("%q: expected arg %d, got %v", input, want, args)
		}
	}
}

func TestDurationOverflowRejected(t *testing.T) {
	cases := []string{
		"totalTime>1e308h",          // huge float, would overflow int on conversion
		"totalTime>999999999h",      // huge but finite decimal hours
		"totalTime>NaNh",            // ParseFloat accepts "NaN"
		"totalTime>Infh",            // ParseFloat accepts "Inf"
		"totalTime>-Infh",           // ParseFloat accepts "-Inf"
		"totalTime>999999999999",    // huge but finite plain minutes, within int64 but past the sane bound
		"totalTime>999999999999:30", // huge hours component in H:MM form
	}
	for _, input := range cases {
		if _, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) should be rejected as an out-of-range duration", input)
		}
	}
}

func TestDurationAtBoundaryAllowed(t *testing.T) {
	// One year in minutes/hours — the documented sane upper bound — should
	// still parse successfully.
	cases := map[string]int{
		"totalTime>8784h":   maxDurationMinutes,
		"totalTime>527040":  maxDurationMinutes,
		"totalTime>8784:00": maxDurationMinutes,
	}
	for input, want := range cases {
		_, args := compile(t, input)
		if len(args) != 1 || args[0] != want {
			t.Errorf("%q: expected arg %d, got %v", input, want, args)
		}
	}
}

func TestIntAndNumberFields(t *testing.T) {
	sql, args := compile(t, "landings>=3 distance>50.5")
	if !strings.Contains(sql, "all_landings >= $1") || !strings.Contains(sql, "distance > $2") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
	if args[0] != 3 || args[1] != 50.5 {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestBoolField(t *testing.T) {
	_, args := compile(t, "isPic:true")
	if args[0] != true {
		t.Fatalf("unexpected args: %v", args)
	}
	_, args = compile(t, "isDual!=yes")
	if args[0] != false {
		t.Fatalf("!= should negate: %v", args)
	}
}

func TestSignedField(t *testing.T) {
	sql, args := compile(t, "signed:true")
	if sql != "signature_id IS NOT NULL" || len(args) != 0 {
		t.Fatalf("unexpected: %s %v", sql, args)
	}
	sql, _ = compile(t, "signed:false")
	if sql != "signature_id IS NULL" {
		t.Fatalf("unexpected: %s", sql)
	}
}

func TestDateRanges(t *testing.T) {
	sql, args := compile(t, "date:2024-05")
	if !strings.Contains(sql, "date >= $1 AND date < $2") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
	start := args[0].(time.Time)
	next := args[1].(time.Time)
	if start.Format("2006-01-02") != "2024-05-01" || next.Format("2006-01-02") != "2024-06-01" {
		t.Fatalf("unexpected range: %v – %v", start, next)
	}

	// date>2024 means strictly after the whole year
	_, args = compile(t, "date>2024")
	if args[0].(time.Time).Format("2006-01-02") != "2025-01-01" {
		t.Fatalf("date>2024 should compare against 2025-01-01, got %v", args[0])
	}
}

func TestClockField(t *testing.T) {
	_, args := compile(t, "offBlock<06:30")
	if args[0] != "06:30:00" {
		t.Fatalf("unexpected clock arg: %v", args)
	}
}

func TestCrewField(t *testing.T) {
	sql, args := compile(t, `crew:"John Doe"`)
	if !strings.Contains(sql, "flight_crew_members") || !strings.Contains(sql, "cm.name ILIKE $1") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
	if args[0] != "%John Doe%" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestApproachTypeField(t *testing.T) {
	sql, _ := compile(t, "approachType:ILS")
	if !strings.Contains(sql, "jsonb_array_elements(approaches)") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
}

func TestBooleanLogic(t *testing.T) {
	sql, args := compile(t, "(departure:EDDF OR arrival:EDDF) AND nightTime>0")
	if !strings.Contains(sql, " OR ") || !strings.Contains(sql, " AND ") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %v", args)
	}
	// OR must be inside the AND grouping
	orIdx := strings.Index(sql, " OR ")
	andIdx := strings.Index(sql, " AND ")
	if orIdx > andIdx {
		t.Fatalf("grouping looks wrong: %s", sql)
	}
}

func TestImplicitAnd(t *testing.T) {
	sql, args := compile(t, "departure:EDDF nightTime>0")
	if !strings.Contains(sql, " AND ") || len(args) != 2 {
		t.Fatalf("adjacent terms should AND: %s %v", sql, args)
	}
}

func TestOrPrecedence(t *testing.T) {
	// a AND b OR c == (a AND b) OR c
	sql, _ := compile(t, "isPic:true nightTime>0 OR isDual:true")
	if !strings.HasPrefix(sql, "((") {
		t.Fatalf("AND should bind tighter than OR: %s", sql)
	}
}

func TestNotVariants(t *testing.T) {
	for _, q := range []string{"NOT remarks:rain", "-remarks:rain", "not remarks:rain"} {
		sql, _ := compile(t, q)
		if !strings.Contains(sql, "NOT ") {
			t.Errorf("%q should compile to NOT: %s", q, sql)
		}
	}
}

func TestArgOffset(t *testing.T) {
	q, err := Parse("departure:EDDF arrival:EDDH")
	if err != nil {
		t.Fatal(err)
	}
	sql, args := q.Compile(5)
	if !strings.Contains(sql, "$5") || !strings.Contains(sql, "$6") || len(args) != 2 {
		t.Fatalf("offset compile wrong: %s %v", sql, args)
	}
}

func TestLikeEscaping(t *testing.T) {
	_, args := compile(t, "remarks:100%_done")
	if args[0] != `%100\%\_done%` {
		t.Fatalf("LIKE metacharacters should be escaped: %v", args)
	}
	_, args = compile(t, "reg:D-E*")
	if args[0] != "%D-E%%" {
		t.Fatalf("* should become %%: %v", args)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"",                        // empty
		"   ",                     // whitespace only
		"bogusField:x",            // unknown field
		"date:notadate",           // bad date
		"totalTime>abc",           // bad duration
		"isPic:maybe",             // bad bool
		"(departure:EDDF",         // unbalanced paren
		"departure:EDDF OR",       // dangling OR
		"NOT",                     // dangling NOT
		"departure:",              // missing value
		"totalTime>>5",            // bad operator use
		`remarks:"unclosed`,       // unterminated quote
		"departure>EDDF",          // range op on text
		"landings:1.5",            // float for int
		"offBlock:25:99",          // bad clock
		strings.Repeat("a", 1001), // too long
	}
	for _, input := range cases {
		if _, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) should fail", input)
		}
	}
}

func TestTermLimit(t *testing.T) {
	q := strings.TrimSpace(strings.Repeat("a ", maxQueryTerms+1))
	if _, err := Parse(q); err == nil {
		t.Fatal("term limit should be enforced")
	}
}

func TestFieldRegistryUnique(t *testing.T) {
	seen := map[string]string{}
	for _, f := range Fields {
		names := append([]string{f.Name}, f.Aliases...)
		for _, n := range names {
			key := strings.ToLower(n)
			if prev, dup := seen[key]; dup {
				t.Errorf("tag %q defined by both %s and %s", n, prev, f.Name)
			}
			seen[key] = f.Name
		}
	}
}
