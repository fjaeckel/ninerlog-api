// Command rulescheck validates every YAML file against schema/ and runs the coverage gate
// (DESIGN.md section 7). It prints a grouped report and exits non-zero on any problem,
// unless -report is given.
//
//	go run ./cmd/rulescheck -strict              # CI gate
//	go run ./cmd/rulescheck -report              # progress report, always exits 0
//	go run ./cmd/rulescheck -report -rule <id>   # one rule only
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

type options struct {
	root        string
	report      bool
	rule        string
	coverage    bool
	minCoverage float64
}

func run(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("rulescheck", flag.ContinueOnError)
	fs.SetOutput(out)
	var o options
	strict := fs.Bool("strict", false, "fail on every problem (the default; spelled out for CI)")
	fs.StringVar(&o.root, "root", ".", "module root")
	fs.BoolVar(&o.report, "report", false, "print the report but exit 0")
	fs.StringVar(&o.rule, "rule", "", "check only this rule id (skips the global sections)")
	fs.BoolVar(&o.coverage, "coverage", true, "measure engine statement coverage with go test")
	fs.Float64Var(&o.minCoverage, "min-coverage", 95, "minimum engine statement coverage in percent")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *strict && o.report {
		fmt.Fprintln(out, "rulescheck: -strict and -report are exclusive")
		return 2
	}
	if o.rule != "" {
		o.coverage = false
	}
	r, err := check(o)
	if err != nil {
		fmt.Fprintf(out, "rulescheck: %v\n", err)
		return 2
	}
	r.print(out, o)
	if r.problems() > 0 && !o.report {
		return 1
	}
	return 0
}
