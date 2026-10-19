package telegram

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"finance-bot/internal/config"
	"finance-bot/internal/finance"
	"finance-bot/internal/storage"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type fakeAPI struct {
	mu       sync.Mutex
	messages []*bot.SendMessageParams
	answers  int
	fail     bool
}

func (f *fakeAPI) SendMessage(_ context.Context, p *bot.SendMessageParams) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, errors.New("offline")
	}
	f.messages = append(f.messages, p)
	return &models.Message{ID: len(f.messages)}, nil
}
func (f *fakeAPI) AnswerCallbackQuery(_ context.Context, _ *bot.AnswerCallbackQueryParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers++
	return true, nil
}
func (f *fakeAPI) last(t *testing.T) *bot.SendMessageParams {
	t.Helper()
	if len(f.messages) == 0 {
		t.Fatal("no messages")
	}
	return f.messages[len(f.messages)-1]
}
func (f *fakeAPI) button(t *testing.T, text string) string {
	t.Helper()
	m, ok := f.last(t).ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatal("no keyboard")
	}
	for _, row := range m.InlineKeyboard {
		for _, b := range row {
			if b.Text == text {
				return b.CallbackData
			}
		}
	}
	t.Fatalf("button %q missing", text)
	return ""
}

type fixture struct {
	a  *App
	s  *storage.Store
	f  *fakeAPI
	id int64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	f := &fakeAPI{}
	a := New(config.Config{AllowedUserID: 42, Location: loc, StateTTL: 15 * time.Minute}, s, f)
	a.now = func() time.Time { return time.Date(2026, 12, 31, 23, 0, 0, 0, loc) }
	return &fixture{a: a, s: s, f: f}
}
func message(id, user int64, kind models.ChatType, text string) *models.Update {
	return &models.Update{ID: id, Message: &models.Message{From: &models.User{ID: user}, Chat: models.Chat{ID: user, Type: kind}, Text: text}}
}
func callback(id, user int64, kind models.ChatType, data string) *models.Update {
	return &models.Update{ID: id, CallbackQuery: &models.CallbackQuery{ID: "cb", From: models.User{ID: user}, Message: models.MaybeInaccessibleMessage{Message: &models.Message{Chat: models.Chat{ID: user, Type: kind}}}, Data: data}}
}
func (x *fixture) msg(text string) {
	x.id++
	x.a.Handle(context.Background(), message(x.id, 42, models.ChatTypePrivate, text))
}
func (x *fixture) click(data string) {
	x.id++
	x.a.Handle(context.Background(), callback(x.id, 42, models.ChatTypePrivate, data))
}
func (x *fixture) choose(t *testing.T, text string) { t.Helper(); x.click(x.f.button(t, text)) }
func (x *fixture) total(t *testing.T) int64 {
	t.Helper()
	r, err := x.s.Summary(context.Background(), "2026-01", "2028-12")
	if err != nil {
		t.Fatal(err)
	}
	return r.Total
}

func TestAuthorization(t *testing.T) {
	x := setup(t)
	x.a.Handle(context.Background(), message(1, 7, models.ChatTypePrivate, "/mes"))
	x.a.Handle(context.Background(), message(2, 42, models.ChatTypeGroup, "/mes"))
	x.a.Handle(context.Background(), message(3, 42, models.ChatTypeSupergroup, "/gasto 10"))
	x.a.Handle(context.Background(), callback(4, 7, models.ChatTypePrivate, "bad"))
	x.a.Handle(context.Background(), callback(5, 42, models.ChatTypeGroup, "bad"))
	x.a.Handle(context.Background(), &models.Update{ID: 6, CallbackQuery: &models.CallbackQuery{From: models.User{ID: 42}}})
	spoof := message(7, 42, models.ChatTypePrivate, "/mes")
	spoof.Message.Chat.ID = 7
	x.a.Handle(context.Background(), spoof)
	if len(x.f.messages) != 0 || x.a.state != nil || x.total(t) != 0 || x.f.answers != 3 {
		t.Fatal("authorization failed")
	}
	x.a.Handle(context.Background(), message(8, 7, models.ChatTypePrivate, "/id"))
	if got := x.f.last(t).Text; got != "Seu Telegram User ID: 7" {
		t.Fatal(got)
	}
}

func TestCashFlowAndDuplicateUpdates(t *testing.T) {
	x := setup(t)
	x.msg("/gasto")
	if x.f.last(t).Text != "Qual foi o valor da despesa?" {
		t.Fatal("amount prompt")
	}
	x.msg("-1")
	if x.a.state.Stage != "amount" {
		t.Fatal("advanced invalid amount")
	}
	x.msg("42,50")
	cat := x.f.button(t, "Mercado")
	x.click(cat)
	// Uma entrega duplicada da categoria não pode repetir a transição.
	x.a.Handle(context.Background(), callback(x.id, 42, models.ChatTypePrivate, cat))
	pay := x.f.button(t, "Pix")
	x.click(pay)
	x.click(pay)
	if x.a.state != nil || x.total(t) != 4250 {
		t.Fatal("cash flow duplicate")
	}
	r, err := x.s.Summary(context.Background(), "2026-12", "2026-12")
	if err != nil || r.Cash != 4250 || r.Credit != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestCreditConfirmationConcurrentAndUndo(t *testing.T) {
	x := setup(t)
	x.msg("/gasto 10")
	x.choose(t, "Alimentação")
	x.choose(t, "Dinheiro")
	x.msg("/gasto 1,00")
	x.choose(t, "Mercado")
	x.choose(t, "Crédito")
	final := x.f.button(t, "3x")
	var wg sync.WaitGroup
	for i := int64(100); i < 110; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			x.a.Handle(context.Background(), callback(id, 42, models.ChatTypePrivate, final))
		}(i)
	}
	wg.Wait()
	x.id = 110
	if x.total(t) != 1100 {
		t.Fatal("double confirmation")
	}
	r, err := x.s.Summary(context.Background(), "2027-01", "2027-03")
	if err != nil || r.Total != 100 || r.Months["2027-01"] != 34 || r.Months["2027-02"] != 33 || r.Months["2027-03"] != 33 {
		t.Fatalf("credit %+v %v", r, err)
	}
	x.msg("/desfazer")
	undo := x.f.button(t, "Sim, desfazer")
	x.click(undo)
	x.click(undo)
	if x.total(t) != 1000 {
		t.Fatal("undo cancelled another purchase")
	}
	x.msg("/mes 01/2027")
	if !strings.Contains(x.f.last(t).Text, "Nenhuma despesa") {
		t.Fatal(x.f.last(t).Text)
	}
}

func TestExpiryRestartCancelAndInvalidCallbacks(t *testing.T) {
	x := setup(t)
	x.msg("/gasto 10")
	old := x.f.button(t, "Mercado")
	x.msg("/gasto 20")
	key := x.a.state.Key
	x.click(old)
	if x.a.state.Key != key || x.a.state.Stage != "category" {
		t.Fatal("old button changed flow")
	}
	x.click(key + ":category:invalid")
	if x.a.state.Stage != "category" {
		t.Fatal("invalid category")
	}
	x.msg("/cancelar")
	x.click(old)
	if x.a.state != nil || x.total(t) != 0 {
		t.Fatal("cancelled flow persisted")
	}
	x.msg("/gasto 10")
	old = x.f.button(t, "Mercado")
	now := x.a.now()
	x.a.now = func() time.Time { return now.Add(16 * time.Minute) }
	x.click(old)
	if x.a.state != nil {
		t.Fatal("no expiration")
	}
	x.msg("/gasto 10")
	old = x.f.button(t, "Mercado")
	lastID := x.id
	x.a = New(x.a.cfg, x.s, x.f)
	x.a.now = func() time.Time { return now }
	x.a.Handle(context.Background(), message(lastID, 42, models.ChatTypePrivate, "/gasto 10"))
	if x.a.state != nil {
		t.Fatal("replayed update after restart")
	}
	x.click(old)
	if x.total(t) != 0 {
		t.Fatal("restart saved old flow")
	}
}

func TestQueriesAndValidation(t *testing.T) {
	x := setup(t)
	for _, cmd := range []string{"/start", "/ajuda", "/mes", "/categorias 08/2026", "/periodo 12/2026 02/2027", "/parcelas", "/desfazer", "/unknown", "/mes 13/2026", "/mes 01/2026 extra", "/periodo 02/2027 01/2026", "/periodo 01/2026", "/gasto x", "/gasto 1 2"} {
		x.msg(cmd)
		if x.f.last(t).Text == "" {
			t.Fatal(cmd)
		}
	}
	x.msg("/gasto 30")
	x.choose(t, "Alimentação")
	x.choose(t, "Débito")
	x.msg("/periodo 12/2026 02/2027")
	text := x.f.last(t).Text
	for _, part := range []string{"Total: R$ 30,00", "Média mensal: R$ 10,00", "01/2027: R$ 0,00", "02/2027: R$ 0,00", "Alimentação: R$ 30,00"} {
		if !strings.Contains(text, part) {
			t.Fatal(text)
		}
	}
	x.msg("/desfazer")
	x.choose(t, "Manter compra")
	if x.total(t) != 3000 {
		t.Fatal("undo no cancelled expense")
	}
}

func TestTooManyInstallmentsAndFailedDelivery(t *testing.T) {
	x := setup(t)
	x.msg("/gasto 0,01")
	x.choose(t, "Outros")
	x.choose(t, "Crédito")
	final := x.f.button(t, "1x")
	x.choose(t, "12x")
	if x.total(t) != 0 || x.a.state.Stage != "installments" {
		t.Fatal("zero cent installments")
	}
	x.f.fail = true
	x.click(final)
	x.f.fail = false
	x.click(final)
	if x.total(t) != 1 || x.a.state != nil {
		t.Fatal("failed send duplicated expense")
	}
}

func TestLocalTimezoneAndSplit(t *testing.T) {
	x := setup(t)
	x.a.now = func() time.Time { return time.Date(2027, 1, 1, 1, 0, 0, 0, time.UTC) }
	x.msg("/gasto 1")
	x.choose(t, "Saúde")
	x.choose(t, "Pix")
	e, err := x.s.Latest(context.Background())
	if err != nil || e.PurchaseDate.Format("2006-01-02") != "2026-12-31" {
		t.Fatal(e, err)
	}
	text := strings.Repeat("Alimentação: "+finance.Money(100)+"\n", 120)
	chunks := splitText(text, 1800)
	if len(chunks) < 2 {
		t.Fatal("not split")
	}
	for _, c := range chunks {
		if len([]rune(c)) > 1800 {
			t.Fatal("oversized chunk")
		}
	}
}
