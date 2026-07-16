package flightsearch

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// sqlBuilder accumulates bind values while the AST is rendered.
type sqlBuilder struct {
	args   []interface{}
	argNum int
}

// bind registers a value and returns its placeholder ("$7").
func (b *sqlBuilder) bind(v interface{}) string {
	ph := fmt.Sprintf("$%d", b.argNum)
	b.args = append(b.args, v)
	b.argNum++
	return ph
}

// Compile renders the query as a parenthesized SQL condition over the flights
// table, with bind placeholders starting at $startArg. It never fails: all
// validation happens in Parse.
func (q *Query) Compile(startArg int) (string, []interface{}) {
	b := &sqlBuilder{argNum: startArg}
	return q.root.sql(b), b.args
}

func (n *andNode) sql(b *sqlBuilder) string { return joinChildren(n.children, " AND ", b) }
func (n *orNode) sql(b *sqlBuilder) string  { return joinChildren(n.children, " OR ", b) }

func (n *notNode) sql(b *sqlBuilder) string {
	return "(NOT " + n.child.sql(b) + ")"
}

func (n *leafNode) sql(b *sqlBuilder) string { return n.fn(b) }

func joinChildren(children []node, sep string, b *sqlBuilder) string {
	parts := make([]string, len(children))
	for i, c := range children {
		parts[i] = c.sql(b)
	}
	return "(" + strings.Join(parts, sep) + ")"
}

// escapeLike escapes LIKE metacharacters and translates '*' wildcards to '%'.
// PostgreSQL's default LIKE escape character is backslash.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	escaped := r.Replace(s)
	return strings.ReplaceAll(escaped, "*", "%")
}

// newFreeTextLeaf builds the condition for a bare term: case-insensitive
// substring match across all free-text columns plus crew member names.
func newFreeTextLeaf(term string) (node, error) {
	pattern := "%" + escapeLike(term) + "%"
	return &leafNode{fn: func(b *sqlBuilder) string {
		ph := b.bind(pattern)
		parts := make([]string, 0, len(freeTextColumns)+1)
		for _, col := range freeTextColumns {
			parts = append(parts, col+" ILIKE "+ph)
		}
		parts = append(parts, "EXISTS (SELECT 1 FROM flight_crew_members cm WHERE cm.flight_id = flights.id AND cm.name ILIKE "+ph+")")
		return "(" + strings.Join(parts, " OR ") + ")"
	}}, nil
}

// newFieldLeaf validates op + value against the field type and builds the
// condition closure.
func newFieldLeaf(field *Field, op, value string) (node, error) {
	switch field.Name {
	case "crew":
		return newCrewLeaf(op, value)
	case "approachType":
		return newApproachTypeLeaf(op, value)
	case "signed":
		return newSignedLeaf(op, value)
	}

	switch field.Type {
	case FieldText:
		return newTextLeaf(field, op, value)
	case FieldDuration:
		minutes, err := parseDurationMinutes(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", field.Name, err)
		}
		return newCompareLeaf(field, op, minutes)
	case FieldInt:
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a whole number", field.Name, value)
		}
		return newCompareLeaf(field, op, n)
	case FieldNumber:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a number", field.Name, value)
		}
		return newCompareLeaf(field, op, f)
	case FieldBool:
		v, err := parseBool(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", field.Name, err)
		}
		return newBoolLeaf(field.Column, op, v)
	case FieldDate:
		start, next, err := parseDateRange(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", field.Name, err)
		}
		return newDateLeaf(field, op, start, next)
	case FieldClock:
		t, err := parseClock(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", field.Name, err)
		}
		return newCompareLeaf(field, op, t)
	}
	return nil, fmt.Errorf("field %q is not searchable", field.Name)
}

func newTextLeaf(field *Field, op, value string) (node, error) {
	col := field.Column
	switch op {
	case ":":
		pattern := "%" + escapeLike(value) + "%"
		return &leafNode{fn: func(b *sqlBuilder) string {
			return col + " ILIKE " + b.bind(pattern)
		}}, nil
	case "=":
		return &leafNode{fn: func(b *sqlBuilder) string {
			return "LOWER(" + col + ") = LOWER(" + b.bind(value) + ")"
		}}, nil
	case "!=":
		return &leafNode{fn: func(b *sqlBuilder) string {
			return "LOWER(" + col + ") <> LOWER(" + b.bind(value) + ")"
		}}, nil
	}
	return nil, fmt.Errorf("%s: operator %q is not valid for text fields (use :, = or !=)", field.Name, op)
}

// newCompareLeaf handles numeric, duration, and clock comparisons.
func newCompareLeaf(field *Field, op string, value interface{}) (node, error) {
	sqlOp, ok := map[string]string{":": "=", "=": "=", "!=": "<>", ">": ">", ">=": ">=", "<": "<", "<=": "<="}[op]
	if !ok {
		return nil, fmt.Errorf("%s: unsupported operator %q", field.Name, op)
	}
	col := field.Column
	return &leafNode{fn: func(b *sqlBuilder) string {
		return col + " " + sqlOp + " " + b.bind(value)
	}}, nil
}

func newBoolLeaf(column, op string, value bool) (node, error) {
	if op != ":" && op != "=" && op != "!=" {
		return nil, fmt.Errorf("boolean fields only support :, = and !=")
	}
	if op == "!=" {
		value = !value
	}
	return &leafNode{fn: func(b *sqlBuilder) string {
		return column + " = " + b.bind(value)
	}}, nil
}

// newDateLeaf compares against a [start, next) range so partial dates
// (YYYY or YYYY-MM) behave intuitively for every operator.
func newDateLeaf(field *Field, op string, start, next time.Time) (node, error) {
	col := field.Column
	build := func(fn func(b *sqlBuilder) string) (node, error) {
		return &leafNode{fn: fn}, nil
	}
	switch op {
	case ":", "=":
		return build(func(b *sqlBuilder) string {
			return "(" + col + " >= " + b.bind(start) + " AND " + col + " < " + b.bind(next) + ")"
		})
	case "!=":
		return build(func(b *sqlBuilder) string {
			return "(" + col + " < " + b.bind(start) + " OR " + col + " >= " + b.bind(next) + ")"
		})
	case ">":
		return build(func(b *sqlBuilder) string { return col + " >= " + b.bind(next) })
	case ">=":
		return build(func(b *sqlBuilder) string { return col + " >= " + b.bind(start) })
	case "<":
		return build(func(b *sqlBuilder) string { return col + " < " + b.bind(start) })
	case "<=":
		return build(func(b *sqlBuilder) string { return col + " < " + b.bind(next) })
	}
	return nil, fmt.Errorf("%s: unsupported operator %q", field.Name, op)
}

func newCrewLeaf(op, value string) (node, error) {
	inner, err := newTextLeaf(&Field{Name: "crew", Column: "cm.name"}, op, value)
	if err != nil {
		return nil, err
	}
	return &leafNode{fn: func(b *sqlBuilder) string {
		return "EXISTS (SELECT 1 FROM flight_crew_members cm WHERE cm.flight_id = flights.id AND " + inner.sql(b) + ")"
	}}, nil
}

func newApproachTypeLeaf(op, value string) (node, error) {
	inner, err := newTextLeaf(&Field{Name: "approachType", Column: "(ap.value->>'type')"}, op, value)
	if err != nil {
		return nil, err
	}
	return &leafNode{fn: func(b *sqlBuilder) string {
		return "EXISTS (SELECT 1 FROM jsonb_array_elements(approaches) ap WHERE " + inner.sql(b) + ")"
	}}, nil
}

func newSignedLeaf(op, value string) (node, error) {
	v, err := parseBool(value)
	if err != nil {
		return nil, fmt.Errorf("signed: %s", err)
	}
	if op == "!=" {
		v = !v
	} else if op != ":" && op != "=" {
		return nil, fmt.Errorf("signed: boolean fields only support :, = and !=")
	}
	cond := "signature_id IS NOT NULL"
	if !v {
		cond = "signature_id IS NULL"
	}
	return &leafNode{fn: func(b *sqlBuilder) string { return cond }}, nil
}

// parseDurationMinutes accepts minutes ("90"), H:MM ("1:30"), or decimal
// hours with an h suffix ("1.5h", "2h") and returns whole minutes.
func parseDurationMinutes(s string) (int, error) {
	if h, m, ok := strings.Cut(s, ":"); ok {
		hours, err1 := strconv.Atoi(h)
		mins, err2 := strconv.Atoi(m)
		if err1 != nil || err2 != nil || hours < 0 || mins < 0 || mins > 59 || len(m) != 2 {
			return 0, fmt.Errorf("%q is not a valid duration (use minutes, H:MM, or hours like 1.5h)", s)
		}
		return hours*60 + mins, nil
	}
	if strings.HasSuffix(strings.ToLower(s), "h") {
		hours, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil || hours < 0 {
			return 0, fmt.Errorf("%q is not a valid duration (use minutes, H:MM, or hours like 1.5h)", s)
		}
		return int(hours*60 + 0.5), nil
	}
	minutes, err := strconv.Atoi(s)
	if err != nil || minutes < 0 {
		return 0, fmt.Errorf("%q is not a valid duration (use minutes, H:MM, or hours like 1.5h)", s)
	}
	return minutes, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	}
	return false, fmt.Errorf("%q is not a boolean (use true or false)", s)
}

// parseDateRange parses YYYY, YYYY-MM, or YYYY-MM-DD into the UTC range
// [start, next) that the value denotes.
func parseDateRange(s string) (time.Time, time.Time, error) {
	var zero time.Time
	if t, err := time.ParseInLocation("2006", s, time.UTC); err == nil {
		return t, t.AddDate(1, 0, 0), nil
	}
	if t, err := time.ParseInLocation("2006-01", s, time.UTC); err == nil {
		return t, t.AddDate(0, 1, 0), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.UTC); err == nil {
		return t, t.AddDate(0, 0, 1), nil
	}
	return zero, zero, fmt.Errorf("%q is not a valid date (use YYYY, YYYY-MM, or YYYY-MM-DD)", s)
}

// parseClock parses HH:MM or HH:MM:SS into a normalized HH:MM:SS string for
// comparison against TIME columns.
func parseClock(s string) (string, error) {
	if t, err := time.Parse("15:04:05", s); err == nil {
		return t.Format("15:04:05"), nil
	}
	if t, err := time.Parse("15:04", s); err == nil {
		return t.Format("15:04:05"), nil
	}
	return "", fmt.Errorf("%q is not a valid time (use HH:MM, UTC)", s)
}
