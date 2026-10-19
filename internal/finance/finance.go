// Package finance contém regras puras: nenhum acesso ao Telegram ou banco.
package finance

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MaxAmountCents int64 = 100_000_000 // R$ 1.000.000,00 por compra

var amountPattern = regexp.MustCompile(`^(?:[0-9]+(?:[,.][0-9]{1,2})?|[1-9][0-9]{0,2}(?:\.[0-9]{3})+,[0-9]{2})$`)

func ParseAmount(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	invalid := errors.New("valor inválido: use 42,50 ou 1.250,90, entre R$ 0,01 e R$ 1.000.000,00")
	if len(s) > 20 || !amountPattern.MatchString(s) {
		return 0, invalid
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	parts := strings.Split(s, ".")
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > MaxAmountCents/100 {
		return 0, invalid
	}
	var cents int64
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		cents, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, invalid
		}
	}
	total := whole*100 + cents
	if total <= 0 || total > MaxAmountCents {
		return 0, invalid
	}
	return total, nil
}

func Money(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	whole := strconv.FormatInt(cents/100, 10)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "." + whole[i:]
	}
	return fmt.Sprintf("%sR$ %s,%02d", sign, whole, cents%100)
}

type Category struct{ ID, Name string }

var Categories = []Category{
	{"food", "Alimentação"}, {"groceries", "Mercado"}, {"transport", "Transporte"},
	{"housing", "Moradia"}, {"health", "Saúde"}, {"leisure", "Lazer"},
	{"shopping", "Compras"}, {"subscriptions", "Assinaturas"}, {"education", "Educação"},
	{"travel", "Viagens"}, {"other", "Outros"},
}

func CategoryName(id string) string {
	for _, c := range Categories {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}
func PaymentName(id string) string {
	switch id {
	case "pix":
		return "Pix"
	case "cash":
		return "Dinheiro"
	case "debit":
		return "Débito"
	case "credit":
		return "Crédito"
	}
	return ""
}

type CreditRules struct{ ClosingDay, DueDay int }

func (r CreditRules) Validate() error {
	if r.ClosingDay == 0 && r.DueDay == 0 {
		return nil
	}
	if r.ClosingDay < 1 || r.ClosingDay > 28 || r.DueDay < 1 || r.DueDay > 28 {
		return errors.New("CREDIT_CLOSING_DAY e CREDIT_DUE_DAY devem ser ambos vazios ou ambos entre 1 e 28")
	}
	return nil
}

type Allocation struct {
	Number         int
	AmountCents    int64
	Month, DueDate string
}
type Expense struct {
	ID                int64
	SourceKey         string
	AmountCents       int64
	Category, Payment string
	PurchaseDate      time.Time
	InstallmentCount  int
	Allocations       []Allocation
}

func NewExpense(key string, amount int64, category, payment string, count int, purchase time.Time, rules CreditRules) (Expense, error) {
	e := Expense{SourceKey: key, AmountCents: amount, Category: category, Payment: payment, PurchaseDate: purchase, InstallmentCount: count}
	if key == "" || amount <= 0 || amount > MaxAmountCents || CategoryName(category) == "" || PaymentName(payment) == "" || purchase.IsZero() || purchase.Year() < 2000 || purchase.Year() > 9998 {
		return e, errors.New("dados da despesa inválidos")
	}
	if err := rules.Validate(); err != nil {
		return e, err
	}
	if count < 1 || count > 12 || int64(count) > amount || (payment != "credit" && count != 1) {
		return e, errors.New("escolha de 1 a 12 parcelas, com pelo menos R$ 0,01 por parcela")
	}
	first := MonthStart(purchase)
	var due time.Time
	if payment == "credit" {
		if rules.ClosingDay == 0 {
			first = first.AddDate(0, 1, 0)
		} else {
			// Compras no dia de fechamento entram no ciclo seguinte, independentemente da hora.
			closing := time.Date(purchase.Year(), purchase.Month(), rules.ClosingDay, 0, 0, 0, 0, purchase.Location())
			if purchase.Day() >= rules.ClosingDay {
				closing = closing.AddDate(0, 1, 0)
			}
			due = time.Date(closing.Year(), closing.Month(), rules.DueDay, 0, 0, 0, 0, purchase.Location())
			if !due.After(closing) {
				due = due.AddDate(0, 1, 0)
			}
			first = MonthStart(due)
		}
	}
	for i := 0; i < count; i++ {
		value := amount / int64(count)
		if int64(i) < amount%int64(count) {
			value++
		}
		a := Allocation{Number: i + 1, AmountCents: value, Month: first.AddDate(0, i, 0).Format("2006-01")}
		if !due.IsZero() {
			a.DueDate = due.AddDate(0, i, 0).Format("2006-01-02")
		}
		e.Allocations = append(e.Allocations, a)
	}
	return e, nil
}

func MonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}
func ParseMonth(s string, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation("01/2006", s, loc)
	if err != nil || t.Format("01/2006") != s || t.Year() < 2000 || t.Year() > 9999 {
		return time.Time{}, errors.New("use MM/AAAA, por exemplo 08/2026 (ano a partir de 2000)")
	}
	return t, nil
}
func Period(start, end time.Time) (int, error) {
	n := (end.Year()-start.Year())*12 + int(end.Month()-start.Month()) + 1
	if n < 1 || n > 120 {
		return 0, errors.New("o período deve estar em ordem e conter no máximo 120 meses")
	}
	return n, nil
}
func MonthName(t time.Time) string {
	names := []string{"Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho", "Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro"}
	return fmt.Sprintf("%s/%04d", names[int(t.Month())-1], t.Year())
}
