// dbtool permite backup consistente e verificação sem token e sem rede.
package main

import (
	"context"
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
	info, err := os.Stat(*database)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("informe um arquivo SQLite existente em -database")
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
