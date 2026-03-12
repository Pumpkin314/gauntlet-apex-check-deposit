package settlement

import (
	"testing"
	"time"
)

func TestCutoffForDate(t *testing.T) {
	// 6:30 PM CT = 00:30 UTC next day (CDT, UTC-5) or 00:30 UTC (CST, UTC-6)
	date := time.Date(2026, 3, 10, 12, 0, 0, 0, CTLocation) // Tuesday March 10
	cutoff := CutoffForDate(date)

	ct := cutoff.In(CTLocation)
	if ct.Hour() != 18 || ct.Minute() != 30 {
		t.Errorf("expected 18:30 CT, got %d:%02d CT", ct.Hour(), ct.Minute())
	}
}

func TestCutoff_Exactly630(t *testing.T) {
	// Deposit at exactly 6:30:00 PM CT → next business day (strictly less than)
	exactly630 := time.Date(2026, 3, 10, CutoffHour, CutoffMinute, 0, 0, CTLocation)
	cutoff := CutoffForDate(exactly630)

	if !exactly630.Equal(cutoff.In(CTLocation)) {
		t.Log("cutoff is the same time, so deposit at exactly 6:30 should NOT be < cutoff")
	}

	// Verify: submitted_at < cutoff should be false for exactly 6:30
	if exactly630.Before(cutoff) {
		t.Errorf("deposit at exactly 18:30 should NOT be before cutoff (strictly less than)")
	}
}

func TestCutoff_Before630(t *testing.T) {
	// Deposit at 6:29:59 PM CT → today's batch
	before := time.Date(2026, 3, 10, 18, 29, 59, 0, CTLocation)
	cutoff := CutoffForDate(before)

	if !before.Before(cutoff) {
		t.Errorf("deposit at 18:29:59 should be before cutoff 18:30:00")
	}
}

func TestCutoff_After630(t *testing.T) {
	// Deposit at 6:30:01 PM CT → next business day
	after := time.Date(2026, 3, 10, 18, 30, 1, 0, CTLocation)
	cutoff := CutoffForDate(after)

	if after.Before(cutoff) {
		t.Errorf("deposit at 18:30:01 should NOT be before cutoff 18:30:00")
	}
}

func TestCutoff_Weekend(t *testing.T) {
	// Saturday deposit → Monday batch (skips Sunday)
	saturday := time.Date(2026, 3, 14, 10, 0, 0, 0, CTLocation) // Saturday
	next := NextBusinessDay(saturday)

	if next.Weekday() != time.Monday {
		t.Errorf("next business day after Saturday should be Monday, got %s", next.Weekday())
	}
	if next.Day() != 16 {
		t.Errorf("expected March 16 (Monday), got March %d", next.Day())
	}
}

func TestCutoff_Holiday(t *testing.T) {
	// Deposit on Christmas 2025 (Thursday) → next business day (Friday Dec 26)
	christmas := time.Date(2025, 12, 25, 10, 0, 0, 0, CTLocation)
	if IsBusinessDay(christmas) {
		t.Errorf("Christmas should not be a business day")
	}

	next := NextBusinessDay(christmas)
	if next.Month() != time.December || next.Day() != 26 {
		t.Errorf("expected Dec 26 as next business day after Christmas 2025, got %s", next.Format("Jan 2"))
	}
}

func TestIsBusinessDay_Weekday(t *testing.T) {
	// Regular Wednesday
	wed := time.Date(2026, 3, 11, 12, 0, 0, 0, CTLocation)
	if !IsBusinessDay(wed) {
		t.Error("regular Wednesday should be a business day")
	}
}

func TestIsBusinessDay_Saturday(t *testing.T) {
	sat := time.Date(2026, 3, 14, 12, 0, 0, 0, CTLocation)
	if IsBusinessDay(sat) {
		t.Error("Saturday should NOT be a business day")
	}
}

func TestIsBusinessDay_Sunday(t *testing.T) {
	sun := time.Date(2026, 3, 15, 12, 0, 0, 0, CTLocation)
	if IsBusinessDay(sun) {
		t.Error("Sunday should NOT be a business day")
	}
}

func TestIsBusinessDay_MLKDay(t *testing.T) {
	// MLK Day 2026: January 19 (3rd Monday)
	mlk := time.Date(2026, 1, 19, 12, 0, 0, 0, CTLocation)
	if IsBusinessDay(mlk) {
		t.Error("MLK Day should NOT be a business day")
	}
}

func TestIsBusinessDay_NewYears(t *testing.T) {
	ny := time.Date(2026, 1, 1, 12, 0, 0, 0, CTLocation)
	if IsBusinessDay(ny) {
		t.Error("New Year's Day should NOT be a business day")
	}
}

func TestCurrentCutoff_BeforeCutoff(t *testing.T) {
	// 2 PM CT on a business day → today's cutoff
	before := time.Date(2026, 3, 10, 14, 0, 0, 0, CTLocation)
	cutoff := CurrentCutoff(before)

	ct := cutoff.In(CTLocation)
	if ct.Day() != 10 || ct.Hour() != 18 || ct.Minute() != 30 {
		t.Errorf("expected March 10 18:30 CT, got %s", ct.Format("Jan 2 15:04"))
	}
}

func TestCurrentCutoff_AfterCutoff(t *testing.T) {
	// 7 PM CT on Tuesday → Wednesday's cutoff
	after := time.Date(2026, 3, 10, 19, 0, 0, 0, CTLocation)
	cutoff := CurrentCutoff(after)

	ct := cutoff.In(CTLocation)
	if ct.Day() != 11 {
		t.Errorf("expected March 11 (next business day), got March %d", ct.Day())
	}
}

func TestBatchCutoffDate_BeforeCutoff(t *testing.T) {
	submitted := time.Date(2026, 3, 10, 14, 0, 0, 0, CTLocation) // 2 PM CT Tuesday
	batchDate := BatchCutoffDate(submitted)

	if batchDate.Day() != 10 {
		t.Errorf("expected March 10 batch, got March %d", batchDate.Day())
	}
}

func TestBatchCutoffDate_AfterCutoff(t *testing.T) {
	submitted := time.Date(2026, 3, 10, 19, 0, 0, 0, CTLocation) // 7 PM CT Tuesday
	batchDate := BatchCutoffDate(submitted)

	if batchDate.Day() != 11 {
		t.Errorf("expected March 11 batch (next business day), got March %d", batchDate.Day())
	}
}
