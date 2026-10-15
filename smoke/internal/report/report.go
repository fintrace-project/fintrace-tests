package report

import (
	"fmt"
	"io"
)

const bodyExcerpt = 200

type Report struct {
	out                     io.Writer
	passed, failed, skipped int
}

func New(out io.Writer) *Report {
	return &Report{out: out}
}

func (r *Report) Section(title string) {
	fmt.Fprintf(r.out, "== %s\n", title)
}

func (r *Report) Note(format string, args ...any) {
	fmt.Fprintf(r.out, "   -- %s\n", fmt.Sprintf(format, args...))
}

func (r *Report) Info(format string, args ...any) {
	fmt.Fprintf(r.out, "   info  %s\n", fmt.Sprintf(format, args...))
}

// Equal compares by printed form, so 57.5 and "57.5" agree; context is shown on failure only.
func (r *Report) Equal(label string, got, want any, context []byte) bool {
	g, w := fmt.Sprint(got), fmt.Sprint(want)
	if g == w {
		r.passed++
		fmt.Fprintf(r.out, "   PASS  %s (%s)\n", label, g)
		return true
	}
	r.failed++
	if len(context) > bodyExcerpt {
		context = context[:bodyExcerpt]
	}
	fmt.Fprintf(r.out, "   FAIL  %s (expected %s, got %s) %s\n", label, w, g, context)
	return false
}

func (r *Report) Skip(label, reason string) {
	r.skipped++
	fmt.Fprintf(r.out, "   SKIP  %s (%s)\n", label, reason)
}

// Summary prints the totals and reports whether the run passed.
func (r *Report) Summary() bool {
	fmt.Fprintf(r.out, "\n== %d passed, %d failed, %d skipped\n", r.passed, r.failed, r.skipped)
	return r.failed == 0
}
