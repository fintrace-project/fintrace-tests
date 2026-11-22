package botreport

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestPeriodStart(t *testing.T) {
	cases := []struct {
		granularity Granularity
		header      string
		want        string
	}{
		{Day, "2024-07-01", "2024-07-01"},
		// A week is labelled by its Sunday and runs from the Monday before it, as Core's buckets do
		{Week, "2024-07-07", "2024-07-01"},
		{Week, "2025-01-05", "2024-12-30"},
		{Month, "JULY - 2024", "2024-07-01"},
		{Month, "DECEMBER - 2024", "2024-12-01"},
		{Year, "2024", "2024-01-01"},
	}
	for _, c := range cases {
		got, err := periodStart(c.granularity, c.header)
		if err != nil || got != c.want {
			t.Errorf("periodStart(%s, %q) = %q, %v; want %q", c.granularity, c.header, got, err, c.want)
		}
	}
}

// Core reads a bucket's balance at `to` plus a day, so a request must end on the last period's last day
func TestLastDay(t *testing.T) {
	cases := []struct {
		granularity Granularity
		start, want string
	}{
		{Day, "2024-07-01", "2024-07-01"},
		{Week, "2024-12-30", "2025-01-05"},
		{Month, "2024-02-01", "2024-02-29"},
		{Month, "2024-12-01", "2024-12-31"},
		{Year, "2024-01-01", "2024-12-31"},
	}
	for _, c := range cases {
		if got := LastDay(c.granularity, c.start); got != c.want {
			t.Errorf("LastDay(%s, %s) = %s, want %s", c.granularity, c.start, got, c.want)
		}
	}
}

func TestPeriodStartRefusesWhatItCannotRead(t *testing.T) {
	for _, c := range []struct {
		granularity Granularity
		header      string
	}{
		{Day, "1 July 2024"},
		{Week, "2024-07-08"}, // a Monday: a week's label is its Sunday
		{Month, "JULY 2024"},
		{Month, "JULLY - 2024"},
		{Year, "24"},
	} {
		if got, err := periodStart(c.granularity, c.header); err == nil {
			t.Errorf("periodStart(%s, %q) = %q, want an error", c.granularity, c.header, got)
		}
	}
}

func TestSplitAccount(t *testing.T) {
	cases := []struct{ label, name, currency string }{
		{"Cash (EUR)", "Cash", "EUR"},
		{"Dinar wallet (RSD)", "Dinar wallet", "RSD"},
		{"Trip (2024) (USD)", "Trip (2024)", "USD"},
	}
	for _, c := range cases {
		name, currency, ok := SplitAccount(c.label)
		if !ok || name != c.name || currency != c.currency {
			t.Errorf("SplitAccount(%q) = %q, %q, %v", c.label, name, currency, ok)
		}
	}
	for _, label := range []string{"Cash", "Cash (EURO)", "(EUR)"} {
		if _, _, ok := SplitAccount(label); ok {
			t.Errorf("SplitAccount(%q) read an account", label)
		}
	}
}

func TestReadKeepsEmptyCellsApartFromZero(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Accounts (per day)"
	if _, err := f.NewSheet(sheet); err != nil {
		t.Fatal(err)
	}
	_ = f.DeleteSheet("Sheet1")
	set := func(cell string, value any) {
		if err := f.SetCellValue(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	set("A1", "Currencies")
	set("B1", "2024-07-01")
	set("C1", "2024-07-02")
	set("D1", "2024-07-03")
	set("A2", "Cash (EUR)")
	set("B2", 137.5)
	set("C2", 0.0)
	set("D2", -55.43)
	// Opened on the third day: nothing before it
	set("A3", "New card (EUR)")
	set("D3", 400.0)
	path := filepath.Join(t.TempDir(), "report.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	report, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sheets) != 1 {
		t.Fatalf("%d sheets, want 1", len(report.Sheets))
	}
	s := report.Sheets[0]
	if s.Scope != Accounts || s.Granularity != Day {
		t.Errorf("scope %s, granularity %s", s.Scope, s.Granularity)
	}
	if want := []string{"2024-07-01", "2024-07-02", "2024-07-03"}; !equal(s.Starts, want) {
		t.Errorf("starts %v, want %v", s.Starts, want)
	}
	cash, card := s.Rows[0], s.Rows[1]
	if cash.Label != "Cash (EUR)" || !is(cash.Values[0], 137.5) || !is(cash.Values[1], 0) || !is(cash.Values[2], -55.43) {
		t.Errorf("cash read as %s %v", cash.Label, show(cash.Values))
	}
	if card.Values[0] != nil || card.Values[1] != nil || !is(card.Values[2], 400) {
		t.Errorf("new card read as %v", show(card.Values))
	}
}

func TestReadsTheBotsReportOfS(t *testing.T) {
	report, err := Read(filepath.Join("..", "..", "..", "dumps", "mok", "S", "references", "mok-bot-report-S.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sheets) != 8 {
		t.Fatalf("%d sheets, want 8", len(report.Sheets))
	}
	day := report.sheet(Accounts, Day)
	if day == nil {
		t.Fatal("no Accounts (per day)")
	}
	// The bot's days stop a day short of S's last, 31 December
	if day.Starts[0] != "2024-07-01" || day.Starts[len(day.Starts)-1] != "2024-12-30" {
		t.Errorf("days from %s to %s", day.Starts[0], day.Starts[len(day.Starts)-1])
	}
	if !is(day.Rows[0].Values[0], 137.5) {
		t.Errorf("Cash on 1 July read as %v", show(day.Rows[0].Values[:1]))
	}
	week := report.sheet(Currencies, Week)
	if week == nil || week.Starts[0] != "2024-07-01" {
		t.Errorf("the first week starts %v", week.Starts[:1])
	}
	year := report.sheet(Currencies, Year)
	if year == nil || len(year.Starts) != 1 || year.Starts[0] != "2024-01-01" {
		t.Errorf("years %v", year.Starts)
	}
}

func (r Report) sheet(scope Scope, granularity Granularity) *Sheet {
	for i := range r.Sheets {
		if r.Sheets[i].Scope == scope && r.Sheets[i].Granularity == granularity {
			return &r.Sheets[i]
		}
	}
	return nil
}

func is(v *float64, want float64) bool { return v != nil && *v == want }

func show(values []*float64) []any {
	out := make([]any, len(values))
	for i, v := range values {
		if v == nil {
			out[i] = nil
		} else {
			out[i] = *v
		}
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
