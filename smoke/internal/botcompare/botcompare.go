// Package botcompare compares the old bot's balances with Core's, period by period.
package botcompare

import "math"

type Result struct {
	// Periods the bot gave a figure for, compared
	Periods int
	// Periods the bot left empty, an account before it existed
	Skipped    int
	Mismatches []Mismatch
}

// Mismatch is a period whose figures differ by more than the tolerance; Core is nil where Core answered no
// bucket for it.
type Mismatch struct {
	Start string
	Bot   float64
	Core  *float64
}

// Row compares one row's figures, aligned with starts, with Core's by bucket start. The tolerance absorbs the
// bot's doubles against Core's exact decimals; a hair above it still differs.
func Row(starts []string, bot []*float64, core map[string]float64, tolerance float64) Result {
	var result Result
	for i, start := range starts {
		if bot[i] == nil {
			result.Skipped++
			continue
		}
		result.Periods++
		figure, ok := core[start]
		if !ok {
			result.Mismatches = append(result.Mismatches, Mismatch{Start: start, Bot: *bot[i]})
			continue
		}
		if math.Abs(figure-*bot[i]) > tolerance+1e-9 {
			result.Mismatches = append(result.Mismatches, Mismatch{Start: start, Bot: *bot[i], Core: &figure})
		}
	}
	return result
}
