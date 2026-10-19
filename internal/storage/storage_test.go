package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"finance-bot/internal/finance"
)

var ctx = context.Background()

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func expense(t *testing.T, key, category, payment, date string, amount int64, count int) finance.Expense {
	t.Helper()
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatal(err)
	}
	e, err := finance.NewExpense(key, amount, category, payment, count, d, finance.CreditRules{})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func save(t *testing.T, s *Store, e finance.Expense) int64 {
	t.Helper()
	id, created, err := s.Save(ctx, e)
	if err != nil || !created {
		t.Fatalf("save: %d %v %v", id, created, err)
	}
	return id
}
func TestReportsAndUndo(t *testing.T) {
	s := testStore(t)
	r, err := s.Summary(ctx, "2026-11", "2026-11")
	if err != nil || r.Total != 0 || len(r.Categories) != 0 {
		t.Fatalf("empty: %+v %v", r, err)
	}
	save(t, s, expense(t, "cash1", "food", "pix", "2026-12-31", 1000, 1))
	save(t, s, expense(t, "cash2", "food", "cash", "2027-01-01", 2000, 1))
	id := save(t, s, expense(t, "credit", "groceries", "credit", "2026-11-30", 10001, 3))
	r, err = s.Summary(ctx, "2026-12", "2027-02")
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 13001 || r.Cash != 3000 || r.Credit != 10001 || r.Categories["food"] != 3000 || r.Categories["groceries"] != 10001 || r.Months["2026-12"] != 4334 || r.Months["2027-01"] != 5334 || r.Months["2027-02"] != 3333 {
		t.Fatalf("report %+v", r)
	}
	future, err := s.Future(ctx, "2027-01")
	if err != nil || len(future) != 2 || future["2027-01"] != 3334 || future["2027-02"] != 3333 {
		t.Fatalf("future %v %v", future, err)
	}
	latest, err := s.Latest(ctx)
	if err != nil || latest.ID != id {
		t.Fatalf("latest %+v %v", latest, err)
	}
	changed, err := s.Cancel(ctx, id, time.Now())
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	changed, err = s.Cancel(ctx, id, time.Now())
	if err != nil || changed {
		t.Fatal("cancel not idempotent", changed, err)
	}
	r, err = s.Summary(ctx, "2026-12", "2027-02")
	if err != nil || r.Total != 3000 || r.Credit != 0 {
		t.Fatalf("cancel report %+v %v", r, err)
	}
	var count int
	if err = s.db.QueryRow("SELECT COUNT(*) FROM allocations WHERE expense_id=?", id).Scan(&count); err != nil || count != 3 {
		t.Fatal("history lost", count, err)
	}
	if err = s.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicFailure(t *testing.T) {
	s := testStore(t)
	_, err := s.db.Exec(`CREATE TRIGGER fail_second BEFORE INSERT ON allocations WHEN NEW.installment_number=2 BEGIN SELECT RAISE(ABORT,'simulated disk failure'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	e := expense(t, "fail", "food", "credit", "2026-12-01", 10000, 3)
	if _, _, err = s.Save(ctx, e); err == nil {
		t.Fatal("wanted failure")
	}
	for _, table := range []string{"expenses", "allocations"} {
		var n int
		if err = s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("partial data %s: %d %v", table, n, err)
		}
	}
	if _, err = s.db.Exec("DROP TRIGGER fail_second"); err != nil {
		t.Fatal(err)
	}
	save(t, s, e)
	if err = s.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentIdempotentSave(t *testing.T) {
	s := testStore(t)
	e := expense(t, "same", "other", "credit", "2026-12-31", 100, 3)
	var wg sync.WaitGroup
	ids := make(chan int64, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _, err := s.Save(ctx, e)
			if err != nil {
				t.Error(err)
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	var first int64
	for id := range ids {
		if first == 0 {
			first = id
		}
		if id != first {
			t.Fatal("multiple IDs")
		}
	}
	r, err := s.Summary(ctx, "2026-01", "2028-01")
	if err != nil || r.Total != 100 {
		t.Fatalf("double count %+v %v", r, err)
	}
	if _, err = s.Cancel(ctx, first, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, created, err := s.Save(ctx, e); err != nil || created {
		t.Fatal("cancelled source recreated", created, err)
	}
}

func TestPersistenceUpdatesAndBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ok, err := s.ClaimUpdate(ctx, 42, now)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	save(t, s, expense(t, "persist", "travel", "debit", "2026-12-31", 12345, 1))
	backup := filepath.Join(dir, "snapshot.db")
	if err = s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err = s.Backup(ctx, backup); err == nil {
		t.Fatal("overwrote backup")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ok, err = s.ClaimUpdate(ctx, 42, now)
	if err != nil || ok {
		t.Fatal("duplicate after restart", ok, err)
	}
	ok, err = s.ClaimUpdate(ctx, 42, now.AddDate(0, 0, 8))
	if err != nil || !ok {
		t.Fatal("expiry", ok, err)
	}
	restored, err := Open(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err = restored.Check(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := restored.Summary(ctx, "2026-12", "2026-12")
	if err != nil || r.Total != 12345 {
		t.Fatalf("restored %+v %v", r, err)
	}
}

func TestConstraintsAndMigration(t *testing.T) {
	s := testStore(t)
	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatal("FK disabled", err)
	}
	if _, err := s.db.Exec(`INSERT INTO allocations(expense_id,installment_number,amount_cents,competence_month) VALUES(999,1,100,'2026-12')`); err == nil {
		t.Fatal("orphan allowed")
	}
	e := expense(t, "bad", "food", "pix", "2026-01-01", 100, 1)
	e.Allocations[0].AmountCents = 99
	if _, _, err := s.Save(ctx, e); err == nil {
		t.Fatal("wrong sum")
	}
	e = expense(t, "bad2", "food", "credit", "2026-01-01", 100, 3)
	e.Allocations[1].Number = 1
	if _, _, err := s.Save(ctx, e); err == nil {
		t.Fatal("duplicate number")
	}
	if _, err := s.db.Exec("PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(ctx); err == nil {
		t.Fatal("accepted future schema")
	}
}
