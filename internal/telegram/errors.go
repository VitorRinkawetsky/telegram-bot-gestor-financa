package telegram

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"strings"

	"github.com/go-telegram/bot"
)

// LogError usa apenas diagnósticos conhecidos. Nunca registra err.Error(): erros
// HTTP podem conter URL com token ou uma resposta que reproduz dados enviados.
func LogError(message string, err error) {
	attrs := []any{"causa", errorCause(err)}
	var limited *bot.TooManyRequestsError
	if errors.As(err, &limited) && limited.RetryAfter > 0 {
		attrs = append(attrs, "tentar_apos_segundos", limited.RetryAfter)
	}
	slog.Warn(message, attrs...)
}

func errorCause(err error) string {
	var dns *net.DNSError
	var certificate *tls.CertificateVerificationError
	var network net.Error
	var limited *bot.TooManyRequestsError
	switch {
	case errors.Is(err, context.Canceled):
		return "operação cancelada"
	case errors.Is(err, context.DeadlineExceeded):
		return "tempo limite de conexão excedido"
	case errors.Is(err, bot.ErrorUnauthorized):
		return "Telegram 401: token inválido ou revogado"
	case errors.Is(err, bot.ErrorForbidden):
		return "Telegram 403: envio proibido; confira se o bot foi bloqueado"
	case errors.Is(err, bot.ErrorBadRequest):
		text := strings.ToLower(err.Error())
		if strings.Contains(text, "markup") || strings.Contains(text, "keyboard") {
			return "Telegram 400: formato do teclado inválido (reply_markup)"
		}
		if strings.Contains(text, "chat not found") {
			return "Telegram 400: conversa não encontrada"
		}
		return "Telegram 400: parâmetros da requisição rejeitados"
	case errors.Is(err, bot.ErrorConflict):
		return "Telegram 409: conflito; confira se há outra instância usando o token"
	case errors.Is(err, bot.ErrorNotFound):
		return "Telegram 404: recurso não encontrado"
	case errors.As(err, &limited), errors.Is(err, bot.ErrorTooManyRequests):
		return "Telegram 429: limite de requisições atingido"
	case errors.As(err, &dns):
		return "falha de resolução DNS"
	case errors.As(err, &certificate):
		return "falha ao verificar certificado TLS"
	case errors.As(err, &network):
		if network.Timeout() {
			return "tempo limite de conexão excedido"
		}
		return "falha de conexão de rede"
	default:
		return "falha de transporte ou resposta inesperada; confira conexão e certificados"
	}
}
