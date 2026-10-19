// Package storage mantém compras e sua distribuição mensal em SQLite.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"finance-bot/internal/finance"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = os.Chmod(abs, 0600); err != nil {
		return nil, err
	}
	p := filepath.ToSlash(abs)
	if filepath.VolumeName(abs) != "" {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	q := url.Values{}
	for _, v := range []string{"foreign_keys(1)", "busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)"} {
		q.Add("_pragma", v)
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return errors.New("schema mais novo que este binário; use a versão correta")
	}
	if version == 0 {
		_, err = tx.ExecContext(ctx, `
CREATE TABLE expenses (
 id INTEGER PRIMARY KEY,
 source_key TEXT NOT NULL UNIQUE,
 amount_cents INTEGER NOT NULL CHECK(amount_cents BETWEEN 1 AND 100000000),
 category TEXT NOT NULL CHECK(category IN ('food','groceries','transport','housing','health','leisure','shopping','subscriptions','education','travel','other')),
 payment_method TEXT NOT NULL CHECK(payment_method IN ('pix','cash','debit','credit')),
 purchase_date TEXT NOT NULL,
 installment_count INTEGER NOT NULL CHECK(installment_count BETWEEN 1 AND 12),
 created_at TEXT NOT NULL,
 cancelled_at TEXT,
 CHECK(payment_method='credit' OR installment_count=1)
);
CREATE TABLE allocations (
 id INTEGER PRIMARY KEY,
 expense_id INTEGER NOT NULL REFERENCES expenses(id),
 installment_number INTEGER NOT NULL CHECK(installment_number BETWEEN 1 AND 12),
 amount_cents INTEGER NOT NULL CHECK(amount_cents>0),
 competence_month TEXT NOT NULL CHECK(length(competence_month)=7),
 due_date TEXT,
 UNIQUE(expense_id,installment_number)
);
CREATE INDEX allocations_month ON allocations(competence_month,expense_id);
CREATE INDEX expenses_active ON expenses(id) WHERE cancelled_at IS NULL;
CREATE TABLE processed_updates (id INTEGER PRIMARY KEY, created_at TEXT NOT NULL);
CREATE INDEX processed_updates_created ON processed_updates(created_at);
PRAGMA user_version=1;`)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClaimUpdate evita repetir mensagens/callbacks mesmo após reiniciar o processo.
// Telegram retém updates por até 24h; guardamos sete dias e não presumimos IDs sequenciais.
func (s *Store) ClaimUpdate(ctx context.Context, id int64, now time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM processed_updates WHERE created_at < ?", now.AddDate(0, 0, -7).UTC().Format(time.RFC3339Nano)); err != nil {
		return false, err
	}
	r, err := tx.ExecContext(ctx, "INSERT INTO processed_updates(id,created_at) VALUES(?,?) ON CONFLICT(id) DO NOTHING", id, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, tx.Commit()
}

func validate(e finance.Expense) error {
	if e.SourceKey == "" || e.AmountCents <= 0 || e.AmountCents > finance.MaxAmountCents || finance.CategoryName(e.Category) == "" || finance.PaymentName(e.Payment) == "" || e.PurchaseDate.IsZero() || e.InstallmentCount < 1 || e.InstallmentCount > 12 || (e.Payment != "credit" && e.InstallmentCount != 1) || len(e.Allocations) != e.InstallmentCount {
		return errors.New("despesa inválida")
	}
	var total int64
	previous := ""
	for i, a := range e.Allocations {
		month, err := time.Parse("2006-01", a.Month)
		if err != nil || month.Format("2006-01") != a.Month || a.Number != i+1 || a.AmountCents <= 0 || a.AmountCents > e.AmountCents {
			return errors.New("distribuição inválida")
		}
		if previous != "" {
			p, _ := time.Parse("2006-01", previous)
			if p.AddDate(0, 1, 0).Format("2006-01") != a.Month {
				return errors.New("meses não consecutivos")
			}
		}
		previous = a.Month
		if a.DueDate != "" {
			due, err := time.Parse("2006-01-02", a.DueDate)
			if err != nil || due.Format("2006-01") != a.Month {
				return errors.New("vencimento inválido")
			}
		}
		if e.Payment != "credit" && (a.Month != e.PurchaseDate.Format("2006-01") || a.DueDate != "") {
			return errors.New("competência à vista inválida")
		}
		total += a.AmountCents
	}
	if total != e.AmountCents {
		return errors.New("parcelas não correspondem ao total")
	}
	return nil
}

// Save grava tudo ou nada. A chave do fluxo é única inclusive para compras canceladas.
func (s *Store) Save(ctx context.Context, e finance.Expense) (int64, bool, error) {
	if err := validate(e); err != nil {
		return 0, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `INSERT INTO expenses(source_key,amount_cents,category,payment_method,purchase_date,installment_count,created_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(source_key) DO NOTHING`, e.SourceKey, e.AmountCents, e.Category, e.Payment, e.PurchaseDate.Format("2006-01-02"), e.InstallmentCount, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, false, err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if n == 0 {
		var id int64
		err = tx.QueryRowContext(ctx, "SELECT id FROM expenses WHERE source_key=?", e.SourceKey).Scan(&id)
		if err != nil {
			return 0, false, err
		}
		return id, false, tx.Commit()
	}
	id, err := r.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	for _, a := range e.Allocations {
		var due any
		if a.DueDate != "" {
			due = a.DueDate
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO allocations(expense_id,installment_number,amount_cents,competence_month,due_date) VALUES(?,?,?,?,?)`, id, a.Number, a.AmountCents, a.Month, due); err != nil {
			return 0, false, err
		}
	}
	return id, true, tx.Commit()
}

type Summary struct {
	Total, Cash, Credit int64
	Categories          map[string]int64
	Months              map[string]int64
}

func (s *Store) Summary(ctx context.Context, from, to string) (Summary, error) {
	r := Summary{Categories: map[string]int64{}, Months: map[string]int64{}}
	rows, err := s.db.QueryContext(ctx, `SELECT a.competence_month,e.category,e.payment_method,SUM(a.amount_cents) FROM allocations a JOIN expenses e ON e.id=a.expense_id WHERE e.cancelled_at IS NULL AND a.competence_month BETWEEN ? AND ? GROUP BY a.competence_month,e.category,e.payment_method ORDER BY a.competence_month,e.category`, from, to)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var month, category, payment string
		var amount int64
		if err = rows.Scan(&month, &category, &payment, &amount); err != nil {
			return r, err
		}
		r.Total += amount
		r.Categories[category] += amount
		r.Months[month] += amount
		if payment == "credit" {
			r.Credit += amount
		} else {
			r.Cash += amount
		}
	}
	return r, rows.Err()
}

func (s *Store) Future(ctx context.Context, from string) (map[string]int64, error) {
	result := map[string]int64{}
	rows, err := s.db.QueryContext(ctx, `SELECT a.competence_month,SUM(a.amount_cents) FROM allocations a JOIN expenses e ON e.id=a.expense_id WHERE e.cancelled_at IS NULL AND e.payment_method='credit' AND a.competence_month>=? GROUP BY a.competence_month ORDER BY a.competence_month`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m string
		var v int64
		if err = rows.Scan(&m, &v); err != nil {
			return nil, err
		}
		result[m] = v
	}
	return result, rows.Err()
}

func (s *Store) Latest(ctx context.Context) (finance.Expense, error) {
	var e finance.Expense
	var date string
	err := s.db.QueryRowContext(ctx, `SELECT id,amount_cents,category,payment_method,purchase_date,installment_count FROM expenses WHERE cancelled_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&e.ID, &e.AmountCents, &e.Category, &e.Payment, &date, &e.InstallmentCount)
	if err != nil {
		return e, err
	}
	e.PurchaseDate, err = time.Parse("2006-01-02", date)
	return e, err
}

// Cancel visa o ID exibido na confirmação, nunca uma nova consulta de "último".
func (s *Store) Cancel(ctx context.Context, id int64, now time.Time) (bool, error) {
	r, err := s.db.ExecContext(ctx, "UPDATE expenses SET cancelled_at=? WHERE id=? AND cancelled_at IS NULL", now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

// Backup usa uma fotografia consistente do SQLite, incluindo o conteúdo do WAL.
// VACUUM INTO recusa um destino existente com dados. A CLI também exige destino inexistente.
func (s *Store) Backup(ctx context.Context, destination string) error {
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		return errors.New("o destino do backup deve ser um arquivo novo")
	}
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", destination)
	if err != nil {
		return err
	}
	return os.Chmod(destination, 0600)
}

func (s *Store) Check(ctx context.Context) error {
	var status string
	if err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&status); err != nil {
		return err
	}
	if status != "ok" {
		return errors.New("SQLite integrity_check falhou")
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	bad := rows.Next()
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return rowErr
	}
	if bad {
		return errors.New("chave estrangeira inválida")
	}
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM expenses e WHERE e.amount_cents != (SELECT COALESCE(SUM(amount_cents),0) FROM allocations WHERE expense_id=e.id) OR e.installment_count != (SELECT COUNT(*) FROM allocations WHERE expense_id=e.id)`).Scan(&count)
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("%d compras inconsistentes", count)
	}
	return nil
}
