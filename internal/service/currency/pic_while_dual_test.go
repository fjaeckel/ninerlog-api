package currency

import "testing"

func TestMetricVal_PICOrDual_CountsOverlapOnce(t *testing.T) {
	p := &Progress{PICMinutes: 200, InstructorMinutes: 150, PICDualOverlapMinutes: 96}
	if got, want := metricVal(p, mPICOrDualMinutes), 254; got != want {
		t.Errorf("metricVal(mPICOrDualMinutes) = %d, want %d", got, want)
	}
}

func TestAddProgress_OverlapFollowsDual(t *testing.T) {
	p := &Progress{PICMinutes: 96, InstructorMinutes: 96, PICDualOverlapMinutes: 96}

	withDual := &Progress{}
	addProgress(withDual, p, true)
	if got := metricVal(withDual, mPICOrDualMinutes); got != 96 {
		t.Errorf("with dual: mPICOrDualMinutes = %d, want 96", got)
	}

	withoutDual := &Progress{}
	addProgress(withoutDual, p, false)
	if withoutDual.PICDualOverlapMinutes != 0 {
		t.Errorf("without dual: PICDualOverlapMinutes = %d, want 0", withoutDual.PICDualOverlapMinutes)
	}
	if got := metricVal(withoutDual, mPICOrDualMinutes); got != 96 {
		t.Errorf("without dual: mPICOrDualMinutes = %d, want 96", got)
	}
}
