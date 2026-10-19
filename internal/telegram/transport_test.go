package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercita a serialização multipart da biblioteca de verdade sem abrir portas,
// usar credenciais reais ou conectar ao Telegram.
func TestTelegramRequestMarkup(t *testing.T) {
	x := setup(t)
	var forms []map[string][]string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		defer r.MultipartForm.RemoveAll()
		forms = append(forms, r.MultipartForm.Value)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":42,"type":"private"}}}`)),
		}, nil
	})}
	b, err := bot.New("123456:"+strings.Repeat("x", 35), bot.WithSkipGetMe(), bot.WithHTTPClient(time.Minute, client))
	if err != nil {
		t.Fatal(err)
	}
	x.a.api = b
	x.a.Handle(context.Background(), message(1, 42, models.ChatTypePrivate, "/id"))
	x.a.Handle(context.Background(), message(2, 42, models.ChatTypePrivate, "/gasto"))
	x.a.Handle(context.Background(), message(3, 42, models.ChatTypePrivate, "42,50"))
	if len(forms) != 3 {
		t.Fatalf("got %d requests", len(forms))
	}
	for i := 0; i < 2; i++ {
		if markup, exists := forms[i]["reply_markup"]; exists {
			t.Errorf("plain message %d must omit reply_markup; got %q", i, markup)
		}
	}
	if got := forms[0]["text"]; len(got) != 1 || got[0] != "Seu Telegram User ID: 42" {
		t.Fatalf("ID response: %v", got)
	}
	var keyboard models.InlineKeyboardMarkup
	if err := json.Unmarshal([]byte(forms[2]["reply_markup"][0]), &keyboard); err != nil {
		t.Fatal(err)
	}
	if len(keyboard.InlineKeyboard) == 0 || keyboard.InlineKeyboard[0][0].CallbackData == "" {
		t.Fatal("category keyboard was lost")
	}
}
