// Package hatches holds escape hatches: named logic the closed vocabulary cannot express
// (DESIGN.md section 5). Each name is declared under `hatches` in vocabulary.yaml.
package hatches

import "math"

// Input is the leaf value the engine computed from the rule's metric and filter.
type Input struct {
	Value   float64
	Tracked bool
}

// Output replaces the leaf's current value and tracked state.
type Output struct {
	Value   float64
	Tracked bool
}

// Func is one escape hatch.
type Func func(Input) Output

// Registry maps hatch names to implementations.
var Registry = map[string]Func{
	"sfcl-130b-credit": sfcl130bCredit,
}

// sfcl130bCredit returns 10 % of the input minutes, at most 420 (SFCL.130(b)).
func sfcl130bCredit(in Input) Output {
	return Output{Value: math.Min(math.Floor(in.Value/10), 420), Tracked: in.Tracked}
}
