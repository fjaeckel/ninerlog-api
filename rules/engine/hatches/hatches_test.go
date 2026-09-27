package hatches

import "testing"

func TestSFCL130bCredit(t *testing.T) {
	for _, tc := range []struct{ in, want float64 }{{0, 0}, {95, 9}, {4200, 420}, {9000, 420}} {
		out := Registry["sfcl-130b-credit"](Input{Value: tc.in, Tracked: true})
		if out.Value != tc.want || !out.Tracked {
			t.Errorf("credit(%v) = %+v, want %v", tc.in, out, tc.want)
		}
	}
	if Registry["sfcl-130b-credit"](Input{Tracked: false}).Tracked {
		t.Error("untracked input must stay untracked")
	}
}
