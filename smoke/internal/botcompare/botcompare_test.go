package botcompare

import "testing"

func f(v float64) *float64 { return &v }

func TestRowWithinToleranceMatches(t *testing.T) {
	starts := []string{"2024-07-01", "2024-07-02"}
	// The bot's doubles against Core's numeric(19,4): a cent apart at most is the same figure
	bot := []*float64{f(1776.0700000000002), f(10.004)}
	core := map[string]float64{"2024-07-01": 1776.07, "2024-07-02": 10.0}

	got := Row(starts, bot, core, 0.01)

	if got.Periods != 2 || got.Skipped != 0 || len(got.Mismatches) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestRowListsEveryMismatchInOrder(t *testing.T) {
	starts := []string{"2024-07-01", "2024-07-02", "2024-07-03"}
	bot := []*float64{f(10), f(20), f(30)}
	core := map[string]float64{"2024-07-01": 10, "2024-07-02": 20.02, "2024-07-03": 29}

	got := Row(starts, bot, core, 0.01)

	if len(got.Mismatches) != 2 {
		t.Fatalf("got %+v", got)
	}
	first := got.Mismatches[0]
	if first.Start != "2024-07-02" || first.Bot != 20 || first.Core == nil || *first.Core != 20.02 {
		t.Errorf("first mismatch %+v", first)
	}
	if got.Mismatches[1].Start != "2024-07-03" {
		t.Errorf("second mismatch %+v", got.Mismatches[1])
	}
}

func TestRowSkipsWhatTheBotLeftEmpty(t *testing.T) {
	starts := []string{"2024-07-01", "2024-07-02"}
	bot := []*float64{nil, f(400)}
	core := map[string]float64{"2024-07-01": 0, "2024-07-02": 400}

	got := Row(starts, bot, core, 0.01)

	if got.Periods != 1 || got.Skipped != 1 || len(got.Mismatches) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestRowReportsAPeriodCoreDidNotAnswer(t *testing.T) {
	got := Row([]string{"2024-07-01"}, []*float64{f(5)}, map[string]float64{}, 0.01)

	if len(got.Mismatches) != 1 || got.Mismatches[0].Core != nil {
		t.Errorf("got %+v", got)
	}
}
