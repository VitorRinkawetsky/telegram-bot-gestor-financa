package finance

import (
	"testing"
	"time"
)

func TestParseAmount(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"42", 4200}, {"42,50", 4250}, {"42.50", 4250}, {"1.250,90", 125090}, {"0,01", 1}, {" 42,5 ", 4250}, {"1.000.000,00", 100000000}, {"1000000", 100000000},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseAmount(tc.in)
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v; want %d", got, err, tc.want)
			}
		})
	}
	for _, in := range []string{"", "0", "0,00", "-1", "+1", "NaN", "1e3", "1,250.90", "1.250", "1,250", "12.34,56", "1.00,00", "1 000", "R$ 42,50", "42,500", "42.", ".50", "1.000.000,01", "1000001", "9999999999999999999999999", "1\n2", "４２", "1_000", "1,2,3"} {
		t.Run("reject_"+in, func(t *testing.T) {
			if got, err := ParseAmount(in); err == nil {
				t.Fatalf("accepted %q: %d", in, got)
			}
		})
	}
}

func TestMoney(t *testing.T) {
	for _, tc := range []struct {
		n int64
		s string
	}{{1, "R$ 0,01"}, {4250, "R$ 42,50"}, {125090, "R$ 1.250,90"}, {100000000, "R$ 1.000.000,00"}, {0, "R$ 0,00"}, {-1, "-R$ 0,01"}} {
		if got := Money(tc.n); got != tc.s {
			t.Fatalf("%d: %s != %s", tc.n, got, tc.s)
		}
	}
}
func localDate(s string) time.Time {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	d, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		panic(err)
	}
	return d
}

func TestInstallments(t *testing.T) {
	for _, tc := range []struct {
		amount int64
		n      int
		want   []int64
	}{{60000, 3, []int64{20000, 20000, 20000}}, {100, 3, []int64{34, 33, 33}}, {5, 3, []int64{2, 2, 1}}, {12, 12, []int64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}}} {
		e, err := NewExpense("key", tc.amount, "food", "credit", tc.n, localDate("2026-12-31"), CreditRules{})
		if err != nil {
			t.Fatal(err)
		}
		var total int64
		for i, a := range e.Allocations {
			total += a.AmountCents
			if a.AmountCents != tc.want[i] || a.Month != localDate("2027-01-01").AddDate(0, i, 0).Format("2006-01") || a.Number != i+1 || a.DueDate != "" {
				t.Fatalf("invalid allocation %+v", a)
			}
		}
		if total != tc.amount {
			t.Fatal("lost cents")
		}
	}
	for _, payment := range []string{"pix", "cash", "debit"} {
		e, err := NewExpense("key", 4250, "groceries", payment, 1, localDate("2026-12-31"), CreditRules{})
		if err != nil || e.Allocations[0].Month != "2026-12" {
			t.Fatalf("cash: %+v, %v", e, err)
		}
	}
}

func TestCreditCalendar(t *testing.T) {
	for _, tc := range []struct {
		purchase     string
		closing, due int
		want         string
	}{
		{"2026-01-19", 20, 10, "2026-02-10"}, {"2026-01-20", 20, 10, "2026-03-10"}, {"2026-01-21", 20, 10, "2026-03-10"},
		{"2026-01-04", 5, 20, "2026-01-20"}, {"2026-01-05", 5, 20, "2026-02-20"},
		{"2026-11-01", 6, 12, "2026-11-12"}, {"2026-11-05", 6, 12, "2026-11-12"},
		{"2026-11-06", 6, 12, "2026-12-12"}, {"2026-11-07", 6, 12, "2026-12-12"},
		{"2026-12-31", 6, 12, "2027-01-12"},
		{"2026-01-04", 5, 5, "2026-02-05"}, {"2026-01-05", 5, 5, "2026-03-05"},
		{"2026-12-28", 28, 1, "2027-02-01"}, {"2028-02-27", 28, 28, "2028-03-28"},
	} {
		t.Run(tc.purchase+tc.want, func(t *testing.T) {
			e, err := NewExpense("k", 1000, "food", "credit", 3, localDate(tc.purchase), CreditRules{tc.closing, tc.due})
			if err != nil {
				t.Fatal(err)
			}
			first := localDate(tc.want)
			for i, a := range e.Allocations {
				if a.DueDate != first.AddDate(0, i, 0).Format("2006-01-02") || a.Month != first.AddDate(0, i, 0).Format("2006-01") {
					t.Fatalf("got %+v", a)
				}
			}
		})
	}
}

func TestExpenseValidation(t *testing.T) {
	for _, r := range []CreditRules{{0, 10}, {10, 0}, {29, 10}, {1, 29}, {-1, 5}} {
		if r.Validate() == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	for _, tc := range []struct {
		amount   int64
		cat, pay string
		n        int
	}{{0, "food", "pix", 1}, {-1, "food", "pix", 1}, {MaxAmountCents + 1, "food", "pix", 1}, {100, "bad", "pix", 1}, {100, "food", "bad", 1}, {100, "food", "pix", 2}, {1, "food", "credit", 2}, {100, "food", "credit", 0}, {100, "food", "credit", 13}} {
		if _, err := NewExpense("k", tc.amount, tc.cat, tc.pay, tc.n, localDate("2026-01-01"), CreditRules{}); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}

func TestMonthsAndPeriod(t *testing.T) {
	for _, in := range []string{"8/2026", "13/2026", "00/2026", "08/26", "08/1999", "2026-08", "08/2026x"} {
		if _, err := ParseMonth(in, time.UTC); err == nil {
			t.Fatalf("accepted %s", in)
		}
	}
	a, _ := ParseMonth("12/2026", time.UTC)
	b, _ := ParseMonth("02/2027", time.UTC)
	if n, err := Period(a, b); n != 3 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
	if _, err := Period(b, a); err == nil {
		t.Fatal("reversed period")
	}
	if _, err := Period(a, a.AddDate(10, 0, 0)); err == nil {
		t.Fatal("unbounded period")
	}
	if MonthName(a) != "Dezembro/2026" {
		t.Fatal(MonthName(a))
	}
}
