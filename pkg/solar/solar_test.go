package solar

import (
	"testing"
	"time"
)

func TestCalculate_Frankfurt_Winter(t *testing.T) {
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	lat, lon := 50.0379, 8.5622
	sun := Calculate(date, lat, lon)
	if sun.Sunrise.Hour() < 6 || sun.Sunrise.Hour() > 9 {
		t.Errorf("Frankfurt winter sunrise at %v, expected 07:20 UTC", sun.Sunrise.Format("15:04"))
	}
	if sun.Sunset.Hour() < 15 || sun.Sunset.Hour() > 17 {
		t.Errorf("Frankfurt winter sunset at %v, expected 16:00 UTC", sun.Sunset.Format("15:04"))
	}
}

func TestCalculate_Frankfurt_Summer(t *testing.T) {
	date := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	lat, lon := 50.0379, 8.5622
	sun := Calculate(date, lat, lon)
	if sun.Sunrise.Hour() < 3 || sun.Sunrise.Hour() > 5 {
		t.Errorf("Frankfurt summer sunrise at %v, expected 03:30 UTC", sun.Sunrise.Format("15:04"))
	}
	if sun.Sunset.Hour() < 19 || sun.Sunset.Hour() > 21 {
		t.Errorf("Frankfurt summer sunset at %v, expected 19:30 UTC", sun.Sunset.Format("15:04"))
	}
}

func TestIsNight_Daytime(t *testing.T) {
	ft := time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC)
	if IsNight(ft, 50.0379, 8.5622) {
		t.Error("14:00 UTC at Frankfurt in January should be daytime")
	}
}

func TestIsNight_Nighttime(t *testing.T) {
	ft := time.Date(2026, 1, 15, 20, 0, 0, 0, time.UTC)
	if !IsNight(ft, 50.0379, 8.5622) {
		t.Error("20:00 UTC at Frankfurt in January should be nighttime")
	}
}

func TestIsNight_EarlyMorning(t *testing.T) {
	ft := time.Date(2026, 1, 15, 5, 0, 0, 0, time.UTC)
	if !IsNight(ft, 50.0379, 8.5622) {
		t.Error("05:00 UTC at Frankfurt in January should be nighttime")
	}
}

func TestCalculate_Equator(t *testing.T) {
	date := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	sun := Calculate(date, 0.0, 0.0)
	if sun.Sunrise.Hour() < 5 || sun.Sunrise.Hour() > 7 {
		t.Errorf("Equator equinox sunrise at %v, expected 06:00 UTC", sun.Sunrise.Format("15:04"))
	}
	if sun.Sunset.Hour() < 17 || sun.Sunset.Hour() > 19 {
		t.Errorf("Equator equinox sunset at %v, expected 18:00 UTC", sun.Sunset.Format("15:04"))
	}
}
