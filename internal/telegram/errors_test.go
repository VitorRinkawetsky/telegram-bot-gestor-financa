package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
)

func TestErrorCause(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w: reply markup must be an object", bot.ErrorBadRequest), "reply_markup"},
		{fmt.Errorf("%w: chat not found", bot.ErrorBadRequest), "conversa não encontrada"},
		{bot.ErrorUnauthorized, "401"}, {bot.ErrorForbidden, "403"}, {bot.ErrorConflict, "409"},
		{&bot.TooManyRequestsError{RetryAfter: 3}, "429"},
		{&url.Error{Op: "Post", URL: "https://example.invalid/secret", Err: &net.DNSError{Err: "no such host"}}, "DNS"},
		{context.DeadlineExceeded, "tempo limite"},
		{errors.New("unrecognized secret"), "resposta inesperada"},
	} {
		if got := errorCause(tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("cause %q, want %q", got, tc.want)
		}
	}
}

func TestErrorLogDoesNotExposeSensitiveDetails(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	const secret = "123456:THIS_IS_A_SECRET_TOKEN"
	err := &url.Error{Op: "Post", URL: "https://api.telegram.org/bot" + secret + "/sendMessage", Err: errors.New("sensitive purchase R$ 42,50")}
	LogError("Falha no envio", err)
	LogError("Falha no envio", &bot.TooManyRequestsError{Message: secret, RetryAfter: 3})
	for _, private := range []string{secret, "42,50", "sensitive purchase", "api.telegram.org"} {
		if strings.Contains(output.String(), private) {
			t.Fatal("sensitive details leaked in log")
		}
	}
	if !strings.Contains(output.String(), `"tentar_apos_segundos":3`) {
		t.Fatal("retry hint missing")
	}
}
