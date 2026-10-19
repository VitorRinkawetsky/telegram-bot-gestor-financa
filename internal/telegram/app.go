package telegram

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"finance-bot/internal/config"
	"finance-bot/internal/finance"
	"finance-bot/internal/storage"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// API é a pequena fronteira de I/O necessária para testar os fluxos sem rede.
type API interface {
	SendMessage(context.Context, *bot.SendMessageParams) (*models.Message, error)
	AnswerCallbackQuery(context.Context, *bot.AnswerCallbackQueryParams) (bool, error)
}
type state struct {
	Key, Stage, Category, Payment string
	Amount                        int64
	Count                         int
	UndoID                        int64
	Expires, Purchase             time.Time
}
type App struct {
	cfg   config.Config
	store *storage.Store
	api   API
	mu    sync.Mutex
	state *state
	now   func() time.Time
}

func New(cfg config.Config, store *storage.Store, api API) *App {
	return &App{cfg: cfg, store: store, api: api, now: time.Now}
}

func Commands() []models.BotCommand {
	return []models.BotCommand{
		{Command: "start", Description: "Boas-vindas e comandos"}, {Command: "ajuda", Description: "Como usar o bot"},
		{Command: "gasto", Description: "Registrar despesa"}, {Command: "mes", Description: "Resumo mensal por competência"},
		{Command: "periodo", Description: "Resumo entre dois meses"}, {Command: "categorias", Description: "Gastos por categoria"},
		{Command: "parcelas", Description: "Previsão dos próximos meses"}, {Command: "desfazer", Description: "Cancelar o último lançamento"},
		{Command: "cancelar", Description: "Cancelar o registro em andamento"}, {Command: "id", Description: "Mostrar seu Telegram User ID"},
	}
}

const help = `Controle suas despesas por aqui:
/gasto — registrar; atalho: /gasto 42,50
/mes — mês atual; exemplo: /mes 08/2026
/periodo 01/2026 06/2026 — período inclusivo (até 120 meses)
/categorias — mês atual; ou /categorias 08/2026
/parcelas — crédito previsto a partir do próximo mês
/desfazer — cancelar a última compra, após confirmação
/cancelar — encerrar o registro em andamento
/id — seu Telegram User ID

À vista entra no mês da compra. Crédito entra nos meses previstos das parcelas. /gasto reinicia o registro. Nenhum dado é salvo antes da última escolha.`

// Handle serializa o processamento. O remetente e a conversa privada são verificados
// antes de ler estado ou banco. /id é a única exceção e só funciona em conversa privada.
func (a *App) Handle(ctx context.Context, u *models.Update) {
	if u == nil {
		return
	}
	if cb := u.CallbackQuery; cb != nil {
		ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if _, err := a.api.AnswerCallbackQuery(ackCtx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID}); err != nil {
			LogError("Não foi possível responder ao botão do Telegram", err)
		}
		cancel()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var chat int64
	if cb := u.CallbackQuery; cb != nil {
		m := cb.Message.Message
		if cb.From.ID != a.cfg.AllowedUserID || m == nil || m.Chat.Type != models.ChatTypePrivate || m.Chat.ID != a.cfg.AllowedUserID {
			return
		}
		chat = m.Chat.ID
	} else if m := u.Message; m != nil {
		if m.From == nil || m.From.IsBot || m.Chat.Type != models.ChatTypePrivate || m.Chat.ID != m.From.ID {
			return
		}
		cmd, _ := command(m.Text)
		if cmd == "id" {
			a.send(ctx, m.Chat.ID, fmt.Sprintf("Seu Telegram User ID: %d", m.From.ID), nil)
			return
		}
		if m.From.ID != a.cfg.AllowedUserID {
			return
		}
		chat = m.Chat.ID
	} else {
		return
	}
	workCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	now := a.now().In(a.cfg.Location)
	ok, err := a.store.ClaimUpdate(workCtx, u.ID, now)
	if err != nil {
		a.failure(workCtx, chat)
		return
	}
	if !ok {
		return
	}
	if a.state != nil && !now.Before(a.state.Expires) {
		a.state = nil
	}
	if u.CallbackQuery != nil {
		a.callback(workCtx, chat, u.CallbackQuery.Data, now)
		return
	}
	cmd, args := command(u.Message.Text)
	if cmd != "" {
		a.handleCommand(workCtx, chat, cmd, args, now)
		return
	}
	if a.state == nil {
		a.send(workCtx, chat, "Use /gasto para registrar uma despesa ou /ajuda para ver os comandos.", nil)
		return
	}
	if a.state.Stage != "amount" {
		a.send(workCtx, chat, "Escolha uma opção nos botões ou use /cancelar.", nil)
		return
	}
	amount, err := finance.ParseAmount(u.Message.Text)
	if err != nil {
		a.send(workCtx, chat, err.Error(), nil)
		return
	}
	a.state.Amount = amount
	a.state.Stage = "category"
	a.state.Expires = now.Add(a.cfg.StateTTL)
	a.categories(workCtx, chat)
}

func command(text string) (string, []string) {
	f := strings.Fields(text)
	if len(f) == 0 || !strings.HasPrefix(f[0], "/") {
		return "", nil
	}
	c := strings.TrimPrefix(strings.SplitN(f[0], "@", 2)[0], "/")
	return strings.ToLower(c), f[1:]
}

func (a *App) handleCommand(ctx context.Context, chat int64, cmd string, args []string, now time.Time) {
	switch cmd {
	case "start":
		a.send(ctx, chat, "Olá! Vamos organizar suas despesas.\n\n"+help, nil)
	case "ajuda":
		a.send(ctx, chat, help, nil)
	case "cancelar":
		a.state = nil
		a.send(ctx, chat, "Fluxo cancelado. Nenhuma compra já salva foi alterada.", nil)
	case "gasto":
		a.state = nil
		if len(args) > 1 {
			a.send(ctx, chat, "Use /gasto ou /gasto 42,50.", nil)
			return
		}
		key, err := newKey()
		if err != nil {
			a.failure(ctx, chat)
			return
		}
		s := &state{Key: key, Stage: "amount", Expires: now.Add(a.cfg.StateTTL)}
		if len(args) == 1 {
			s.Amount, err = finance.ParseAmount(args[0])
			if err != nil {
				a.send(ctx, chat, err.Error(), nil)
				return
			}
			s.Stage = "category"
		}
		a.state = s
		if s.Stage == "amount" {
			a.send(ctx, chat, "Qual foi o valor da despesa?", nil)
		} else {
			a.categories(ctx, chat)
		}
	case "mes", "categorias":
		month := finance.MonthStart(now)
		if len(args) > 1 {
			a.send(ctx, chat, "Use /"+cmd+" ou /"+cmd+" 08/2026.", nil)
			return
		}
		if len(args) == 1 {
			var err error
			month, err = finance.ParseMonth(args[0], a.cfg.Location)
			if err != nil {
				a.send(ctx, chat, err.Error(), nil)
				return
			}
		}
		r, err := a.store.Summary(ctx, month.Format("2006-01"), month.Format("2006-01"))
		if err != nil {
			a.failure(ctx, chat)
			return
		}
		text := "Resumo de " + finance.MonthName(month) + "\nTotal: " + finance.Money(r.Total)
		if r.Total == 0 {
			text += "\nNenhuma despesa neste mês."
		} else {
			text += categoryTotals(r.Categories)
			if cmd == "mes" {
				text += "\nÀ vista: " + finance.Money(r.Cash) + "\nParcelas de crédito: " + finance.Money(r.Credit)
			}
		}
		a.send(ctx, chat, text, nil)
	case "periodo":
		if len(args) != 2 {
			a.send(ctx, chat, "Use /periodo 01/2026 06/2026.", nil)
			return
		}
		from, err := finance.ParseMonth(args[0], a.cfg.Location)
		if err != nil {
			a.send(ctx, chat, err.Error(), nil)
			return
		}
		to, err := finance.ParseMonth(args[1], a.cfg.Location)
		if err != nil {
			a.send(ctx, chat, err.Error(), nil)
			return
		}
		n, err := finance.Period(from, to)
		if err != nil {
			a.send(ctx, chat, err.Error(), nil)
			return
		}
		r, err := a.store.Summary(ctx, from.Format("2006-01"), to.Format("2006-01"))
		if err != nil {
			a.failure(ctx, chat)
			return
		}
		// Arredonda somente a apresentação da média; os totais continuam inteiros exatos.
		text := fmt.Sprintf("Período %s a %s\nTotal: %s\nMédia mensal: %s", args[0], args[1], finance.Money(r.Total), finance.Money((r.Total+int64(n)/2)/int64(n)))
		if r.Total == 0 {
			text += "\nNenhuma despesa neste período."
		} else {
			text += categoryTotals(r.Categories)
		}
		for m := from; !m.After(to); m = m.AddDate(0, 1, 0) {
			text += "\n" + m.Format("01/2006") + ": " + finance.Money(r.Months[m.Format("2006-01")])
		}
		a.send(ctx, chat, text, nil)
	case "parcelas":
		if len(args) != 0 {
			a.send(ctx, chat, "Use /parcelas sem argumentos.", nil)
			return
		}
		from := finance.MonthStart(now).AddDate(0, 1, 0)
		months, err := a.store.Future(ctx, from.Format("2006-01"))
		if err != nil {
			a.failure(ctx, chat)
			return
		}
		keys := make([]string, 0, len(months))
		for k := range months {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		text := "Parcelas previstas a partir de " + from.Format("01/2006")
		var total int64
		for _, k := range keys {
			m, _ := time.Parse("2006-01", k)
			text += "\n" + m.Format("01/2006") + ": " + finance.Money(months[k])
			total += months[k]
		}
		if total == 0 {
			text += "\nNenhuma parcela futura."
		} else {
			text += "\nTotal futuro: " + finance.Money(total)
		}
		a.send(ctx, chat, text, nil)
	case "desfazer":
		if len(args) != 0 {
			a.send(ctx, chat, "Use /desfazer sem argumentos.", nil)
			return
		}
		e, err := a.store.Latest(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			a.send(ctx, chat, "Não há lançamento ativo para desfazer.", nil)
			return
		}
		if err != nil {
			a.failure(ctx, chat)
			return
		}
		key, err := newKey()
		if err != nil {
			a.failure(ctx, chat)
			return
		}
		a.state = &state{Key: key, Stage: "undo", UndoID: e.ID, Expires: now.Add(a.cfg.StateTTL)}
		a.send(ctx, chat, fmt.Sprintf("Cancelar a compra #%d de %s (%s), feita em %s, incluindo todas as parcelas? Um registro incompleto anterior foi encerrado.", e.ID, finance.Money(e.AmountCents), finance.CategoryName(e.Category), e.PurchaseDate.Format("02/01/2006")), a.keyboard([]choice{{"Sim, desfazer", "undo", "yes"}, {"Manter compra", "undo", "no"}}, 2))
	default:
		a.send(ctx, chat, "Comando desconhecido. Use /ajuda.", nil)
	}
}

func (a *App) callback(ctx context.Context, chat int64, data string, now time.Time) {
	p := strings.Split(data, ":")
	if len(p) != 3 || a.state == nil || p[0] != a.state.Key || p[1] != a.state.Stage {
		a.send(ctx, chat, "Este botão expirou ou já foi usado. Use /gasto ou /desfazer para começar novamente.", nil)
		return
	}
	s := a.state
	s.Expires = now.Add(a.cfg.StateTTL)
	switch s.Stage {
	case "category":
		if finance.CategoryName(p[2]) == "" {
			a.invalid(ctx, chat)
			return
		}
		s.Category = p[2]
		s.Stage = "payment"
		a.send(ctx, chat, "Como você pagou?", a.keyboard([]choice{{"Pix", "payment", "pix"}, {"Dinheiro", "payment", "cash"}, {"Débito", "payment", "debit"}, {"Crédito", "payment", "credit"}}, 2))
	case "payment":
		if finance.PaymentName(p[2]) == "" {
			a.invalid(ctx, chat)
			return
		}
		if !s.Purchase.IsZero() && p[2] != s.Payment {
			a.send(ctx, chat, "Tente o mesmo método de pagamento ou use /cancelar e confira /mes.", nil)
			return
		}
		s.Payment = p[2]
		if s.Payment == "credit" {
			s.Stage = "installments"
			choices := []choice{}
			for i := 1; i <= 12; i++ {
				n := strconv.Itoa(i)
				choices = append(choices, choice{n + "x", "installments", n})
			}
			a.send(ctx, chat, "Em quantas vezes?", a.keyboard(choices, 4))
			return
		}
		a.save(ctx, chat, 1, now)
	case "installments":
		n, err := strconv.Atoi(p[2])
		if err != nil || n < 1 || n > 12 {
			a.invalid(ctx, chat)
			return
		}
		a.save(ctx, chat, n, now)
	case "undo":
		if p[2] == "no" {
			a.state = nil
			a.send(ctx, chat, "Compra mantida.", nil)
			return
		}
		if p[2] != "yes" {
			a.invalid(ctx, chat)
			return
		}
		changed, err := a.store.Cancel(ctx, s.UndoID, now)
		if err != nil {
			a.failure(ctx, chat)
			return
		}
		a.state = nil
		if changed {
			a.send(ctx, chat, "Compra cancelada, incluindo suas parcelas. O histórico foi preservado.", nil)
		} else {
			a.send(ctx, chat, "Esta compra já estava cancelada.", nil)
		}
	default:
		a.invalid(ctx, chat)
	}
}

func (a *App) save(ctx context.Context, chat int64, count int, now time.Time) {
	s := a.state
	if !s.Purchase.IsZero() && count != s.Count {
		a.send(ctx, chat, "Tente a mesma quantidade de parcelas ou use /cancelar e confira /mes.", nil)
		return
	}
	purchase := s.Purchase
	if purchase.IsZero() {
		purchase = now
	}
	e, err := finance.NewExpense(s.Key, s.Amount, s.Category, s.Payment, count, purchase, a.cfg.Credit)
	if err != nil {
		a.send(ctx, chat, err.Error(), nil)
		return
	}
	s.Purchase = purchase
	s.Count = count
	id, _, err := a.store.Save(ctx, e)
	if err != nil {
		a.failure(ctx, chat)
		return
	}
	a.state = nil
	text := fmt.Sprintf("Despesa #%d registrada: %s · %s · %s\nCompra: %s", id, finance.Money(e.AmountCents), finance.CategoryName(e.Category), finance.PaymentName(e.Payment), e.PurchaseDate.Format("02/01/2006"))
	if e.Payment == "credit" {
		text += fmt.Sprintf("\n%d parcela(s):", count)
		for _, p := range e.Allocations {
			m, _ := time.Parse("2006-01", p.Month)
			text += fmt.Sprintf("\n%d/%d — %s: %s", p.Number, count, m.Format("01/2006"), finance.Money(p.AmountCents))
		}
		if a.cfg.Credit.ClosingDay == 0 {
			text += "\nPrevisão simplificada; pode diferir da fatura do banco."
		}
	}
	a.send(ctx, chat, text, nil)
}

type choice struct{ Text, Action, Value string }

func (a *App) keyboard(choices []choice, cols int) *models.InlineKeyboardMarkup {
	rows := [][]models.InlineKeyboardButton{}
	for i, c := range choices {
		if i%cols == 0 {
			rows = append(rows, []models.InlineKeyboardButton{})
		}
		j := len(rows) - 1
		rows[j] = append(rows[j], models.InlineKeyboardButton{Text: c.Text, CallbackData: a.state.Key + ":" + c.Action + ":" + c.Value})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
func (a *App) categories(ctx context.Context, chat int64) {
	choices := []choice{}
	for _, c := range finance.Categories {
		choices = append(choices, choice{c.Name, "category", c.ID})
	}
	a.send(ctx, chat, "Escolha a categoria de "+finance.Money(a.state.Amount)+":", a.keyboard(choices, 2))
}
func categoryTotals(totals map[string]int64) string {
	text := "\nPor categoria:"
	for _, c := range finance.Categories {
		if v := totals[c.ID]; v != 0 {
			text += "\n• " + c.Name + ": " + finance.Money(v)
		}
	}
	return text
}
func newKey() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func (a *App) invalid(ctx context.Context, chat int64) {
	a.send(ctx, chat, "Opção inválida. Use os botões da etapa atual.", nil)
}
func (a *App) failure(ctx context.Context, chat int64) {
	slog.Error("Falha na operação do banco; verifique armazenamento e permissões")
	a.send(ctx, chat, "Não foi possível concluir. Confira /mes antes de tentar novamente. Se estava registrando, tente o mesmo botão ou reinicie com /gasto.", nil)
}
func (a *App) send(ctx context.Context, chat int64, text string, markup *models.InlineKeyboardMarkup) {
	// O limite é aplicado em runas, com margem para UTF-16 usado pelo Telegram.
	for i, chunk := range splitText(text, 1800) {
		// Relatórios longos respeitam o ritmo de envio por conversa do Telegram.
		if i > 0 {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		params := &bot.SendMessageParams{ChatID: chat, Text: chunk}
		// ReplyMarkup é uma interface: atribuir um ponteiro nil produziria
		// reply_markup=null no multipart. Mensagens sem teclado devem omitir o campo.
		if markup != nil {
			params.ReplyMarkup = markup
		}
		if _, err := a.api.SendMessage(ctx, params); err != nil {
			LogError("Não foi possível enviar mensagem pelo Telegram", err)
			return
		}
	}
}
func splitText(text string, limit int) []string {
	var chunks []string
	var current []rune
	for _, line := range strings.Split(text, "\n") {
		r := []rune(line)
		if len(current)+len(r)+1 > limit && len(current) > 0 {
			chunks = append(chunks, string(current))
			current = nil
		}
		for len(r) > limit {
			chunks = append(chunks, string(r[:limit]))
			r = r[limit:]
		}
		if len(current) > 0 {
			current = append(current, '\n')
		}
		current = append(current, r...)
	}
	if len(current) > 0 {
		chunks = append(chunks, string(current))
	}
	return chunks
}
