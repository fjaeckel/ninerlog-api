package models

import (
	"errors"
	"testing"
)

// Declared function times (PICUS/SPIC/relief) join the function-time sum;
// examiner time overlays it.

func TestValidateTimeDistribution_PICUSWithinTotal(t *testing.T) {
	f := validFlight()
	f.PICTime = 0
	f.PICUSTime = f.TotalTime
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}
}

func TestValidateTimeDistribution_FunctionSumExceedsTotal(t *testing.T) {
	f := validFlight()
	f.PICTime = f.TotalTime
	f.PICUSTime = 1
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidFunctionTime) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrInvalidFunctionTime", err)
	}
}

func TestValidateTimeDistribution_SPICAndReliefInSum(t *testing.T) {
	f := validFlight()
	f.PICTime = 0
	f.SPICTime = f.TotalTime / 2
	f.ReliefTime = f.TotalTime - f.SPICTime
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}
	f.ReliefTime++
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidFunctionTime) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrInvalidFunctionTime", err)
	}
}

func TestValidateTimeDistribution_ExaminerOverlays(t *testing.T) {
	f := validFlight()
	f.ExaminerTime = f.TotalTime
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil (examiner overlays function time)", err)
	}
	f.ExaminerTime = f.TotalTime + 1
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidExaminerTime) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrInvalidExaminerTime", err)
	}
}

func TestValidateTimeDistribution_NegativeFunctionTimes(t *testing.T) {
	for _, set := range []func(*Flight){
		func(f *Flight) { f.PICUSTime = -1 },
		func(f *Flight) { f.SPICTime = -1 },
		func(f *Flight) { f.ExaminerTime = -1 },
		func(f *Flight) { f.ReliefTime = -1 },
	} {
		f := validFlight()
		set(f)
		if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrNegativeTime) {
			t.Errorf("ValidateTimeDistribution() = %v, want ErrNegativeTime", err)
		}
	}
}

func TestValidateTimeDistribution_PassengerRejectsFunctionTimes(t *testing.T) {
	f := validFlight()
	f.IsPassenger = true
	f.TotalTime = 0
	f.PICTime = 0
	f.PICUSTime = 30
	if err := f.ValidateTimeDistribution(); err == nil {
		t.Error("ValidateTimeDistribution() = nil, want error (passenger flight logs no function time)")
	}
}

func TestValidateSessionTimes_RejectsFunctionTimes(t *testing.T) {
	fstd := "FNPT II"
	f := &Flight{IsSimulator: true, FSTDType: &fstd, SimulatedFlightTime: 60, SPICTime: 30}
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrFSTDFlightTime) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrFSTDFlightTime", err)
	}
}
