// Package botreport reads the old MOK bot's xlsx report: balances per currency and per account, each over
// days, weeks, months and years, one sheet each.
package botreport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type Scope string

const (
	Currencies Scope = "Currencies"
	Accounts   Scope = "Accounts"
)

// Granularity is spelled as Core's `granularity` parameter.
type Granularity string

const (
	Day   Granularity = "DAY"
	Week  Granularity = "WEEK"
	Month Granularity = "MONTH"
	Year  Granularity = "YEAR"
)

type Report struct {
	Sheets []Sheet
}

// Sheet holds one scope over one granularity. Starts are the periods' first days, as Core's buckets are
// identified; each row's Values are aligned with them, nil where the bot wrote nothing — an account before
// it existed.
type Sheet struct {
	Name        string
	Scope       Scope
	Granularity Granularity
	Starts      []string
	Rows        []Row
}

// Row is a currency (`EUR`) or an account (`Cash (EUR)`) and its closing balance per period.
type Row struct {
	Label  string
	Values []*float64
}

var sheetName = regexp.MustCompile(`^(Currencies|Accounts) \(per (day|week|month|year)\)$`)

var granularities = map[string]Granularity{"day": Day, "week": Week, "month": Month, "year": Year}

// Read opens the report and reads every sheet it recognises by name; any other sheet is an error, so a
// changed report is noticed rather than half compared.
func Read(path string) (Report, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = f.Close() }()

	var report Report
	for _, name := range f.GetSheetList() {
		m := sheetName.FindStringSubmatch(name)
		if m == nil {
			return Report{}, fmt.Errorf("%s: unexpected sheet %q", path, name)
		}
		// Raw values: the bot's doubles as stored, not as the sheet's number format displays them
		rows, err := f.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", name, err)
		}
		sheet, err := readSheet(name, Scope(m[1]), granularities[m[2]], rows)
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", name, err)
		}
		report.Sheets = append(report.Sheets, sheet)
	}
	return report, nil
}

func readSheet(name string, scope Scope, granularity Granularity, rows [][]string) (Sheet, error) {
	sheet := Sheet{Name: name, Scope: scope, Granularity: granularity}
	if len(rows) == 0 {
		return sheet, fmt.Errorf("empty")
	}
	for _, header := range rows[0][1:] {
		start, err := periodStart(granularity, header)
		if err != nil {
			return sheet, err
		}
		sheet.Starts = append(sheet.Starts, start)
	}
	for i, cells := range rows[1:] {
		if len(cells) == 0 || cells[0] == "" {
			continue
		}
		row := Row{Label: cells[0], Values: make([]*float64, len(sheet.Starts))}
		// GetRows drops a row's trailing empty cells, so a short row is one that ends early
		for j := 1; j < len(cells) && j <= len(sheet.Starts); j++ {
			if cells[j] == "" {
				continue
			}
			v, err := strconv.ParseFloat(cells[j], 64)
			if err != nil {
				return sheet, fmt.Errorf("row %d, %s: %w", i+2, sheet.Starts[j-1], err)
			}
			row.Values[j-1] = &v
		}
		sheet.Rows = append(sheet.Rows, row)
	}
	return sheet, nil
}

// periodStart turns a column's header into its period's first day. The bot labels a week by its Sunday, a
// month as `JULY - 2024`, a year as `2024`.
func periodStart(granularity Granularity, header string) (string, error) {
	const iso = "2006-01-02"
	switch granularity {
	case Day:
		d, err := time.Parse(iso, header)
		if err != nil {
			return "", fmt.Errorf("day %q: %w", header, err)
		}
		return d.Format(iso), nil
	case Week:
		d, err := time.Parse(iso, header)
		if err != nil {
			return "", fmt.Errorf("week %q: %w", header, err)
		}
		if d.Weekday() != time.Sunday {
			return "", fmt.Errorf("week %q is a %s, not the Sunday it ends on", header, d.Weekday())
		}
		return d.AddDate(0, 0, -6).Format(iso), nil
	case Month:
		d, err := time.Parse("January - 2006", titleCase(header))
		if err != nil {
			return "", fmt.Errorf("month %q: %w", header, err)
		}
		return d.Format(iso), nil
	case Year:
		d, err := time.Parse("2006", header)
		if err != nil || len(header) != 4 {
			return "", fmt.Errorf("year %q", header)
		}
		return d.Format(iso), nil
	}
	return "", fmt.Errorf("granularity %q", granularity)
}

// LastDay is the last day of the period starting on start.
func LastDay(granularity Granularity, start string) string {
	d, err := time.Parse("2006-01-02", start)
	if err != nil {
		return start
	}
	next := map[Granularity]time.Time{
		Day:   d.AddDate(0, 0, 1),
		Week:  d.AddDate(0, 0, 7),
		Month: d.AddDate(0, 1, 0),
		Year:  d.AddDate(1, 0, 0),
	}[granularity]
	return next.AddDate(0, 0, -1).Format("2006-01-02")
}

// titleCase makes the bot's `JULY - 2024` the `July - 2024` Go's layout reads.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return s[:1] + strings.ToLower(s[1:])
}

var accountLabel = regexp.MustCompile(`^(.+) \(([A-Z]{3})\)$`)

// SplitAccount reads `Name (CUR)`, the bot's label for an account.
func SplitAccount(label string) (name, currency string, ok bool) {
	m := accountLabel.FindStringSubmatch(label)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}
