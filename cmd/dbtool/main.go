// dbtool permite backup consistente e verificação sem token e sem rede.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"finance-bot/internal/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	action := flag.String("action", "check", "check ou backup")
	database := flag.String("database", "", "arquivo SQLite existente")
	out := flag.String("out", "", "novo arquivo de backup")
	flag.Parse()
	if *action != "check" && *action != "backup" {
		return fmt.Errorf("ação inválida: use check ou backup")
	}
	if err := checkDatabasePath(*database); err != nil {
		return err
	}
	if *action == "backup" && *out == "" {
		return fmt.Errorf("informe um arquivo novo em -out")
	}
	s, err := storage.Open(*database)
	if err != nil {
		return fmt.Errorf("falha ao abrir banco: %w", err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = s.Check(ctx); err != nil {
		return err
	}
	if *action == "backup" {
		if err = s.Backup(ctx, *out); err != nil {
			return err
		}
		fmt.Println("Backup consistente criado. Copie-o para fora da VM e verifique a restauração.")
	} else {
		fmt.Println("SQLite, chaves estrangeiras e totais das compras: OK")
	}
	return nil
}

// Diferencia arquivo inexistente de acesso negado para não sugerir a criação
// de outro banco quando o arquivo original existe, mas não pode ser acessado.
func checkDatabasePath(path string) error {
	if path == "" {
		return errors.New("informe o caminho do SQLite existente em -database")
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("banco SQLite não encontrado em %q; confira DATABASE_PATH e o diretório de trabalho do serviço: %w", path, err)
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("sem permissão para acessar o SQLite em %q; confira acesso ao arquivo e aos diretórios: %w", path, err)
	}
	if err != nil {
		return fmt.Errorf("não foi possível verificar o SQLite em %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q não é um arquivo SQLite regular", path)
	}
	return nil
}
