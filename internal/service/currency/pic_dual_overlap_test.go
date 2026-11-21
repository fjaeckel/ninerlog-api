package currency

import "testing"

func TestPICOrDualDoesNotDoubleCountOverlappingCredits(t *testing.T) {
	p := &Progress{PICMinutes: 66, InstructorMinutes: 90, OverlappingPICDualMinutes: 66}
	if got := metricVal(p, mPICOrDualMinutes); got != 90 {
		t.Fatalf("credit=%d, want 90", got)
	}
	sum := &Progress{}
	addProgress(sum, p, true)
	addProgress(sum, &Progress{PICMinutes: 30}, true)
	if got := metricVal(sum, mPICOrDualMinutes); got != 120 {
		t.Fatalf("merged credit=%d, want 120", got)
	}
	withoutDual := &Progress{}
	addProgress(withoutDual, p, false)
	if got := metricVal(withoutDual, mPICOrDualMinutes); got != 66 {
		t.Fatalf("PIC-only credit=%d", got)
	}
}
