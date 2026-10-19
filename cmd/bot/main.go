package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"finance-bot/internal/config"
	"finance-bot/internal/storage"
	"finance-bot/internal/telegram"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	store, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("Não foi possível abrir o SQLite; confira caminho e permissões")
		return safeError("inicialização do banco falhou")
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var app *telegram.App
	b, err := bot.New(cfg.Token, bot.WithDefaultHandler(func(ctx context.Context, _ *bot.Bot, u *models.Update) { app.Handle(ctx, u) }), bot.WithWorkers(1), bot.WithNotAsyncHandlers(), bot.WithUpdatesChannelCap(1), bot.WithAllowedUpdates(bot.AllowedUpdates{"message", "callback_query"}), bot.WithErrorsHandler(func(err error) {
		telegram.LogError("Falha na API Telegram; nova tentativa automática", err)
	}))
	if err != nil {
		return safeError("não foi possível validar o bot no Telegram; verifique token e conexão")
	}
	app = telegram.New(cfg, store, b)
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err = b.DeleteWebhook(initCtx, &bot.DeleteWebhookParams{DropPendingUpdates: false}); err != nil {
		return safeError("não foi possível habilitar long polling no Telegram")
	}
	if _, err = b.SetMyCommands(initCtx, &bot.SetMyCommandsParams{Commands: telegram.Commands()}); err != nil {
		return safeError("não foi possível configurar o menu de comandos do Telegram")
	}
	slog.Info("Bot iniciado com long polling; acesso restrito e sem portas de entrada")
	b.Start(ctx)
	slog.Info("Bot encerrado")
	return nil
}

type safeError string

func (e safeError) Error() string { return string(e) }
