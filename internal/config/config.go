package config

import (
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Funciona também em hosts sem o pacote tzdata.

	"finance-bot/internal/finance"
)

type Config struct {
	Token         string
	AllowedUserID int64
	DatabasePath  string
	Location      *time.Location
	StateTTL      time.Duration
	Credit        finance.CreditRules
}

func Load() (Config, error) { return Read(os.Getenv) }
func Read(get func(string) string) (Config, error) {
	var c Config
	c.Token = strings.TrimSpace(get("TELEGRAM_BOT_TOKEN"))
	if !regexp.MustCompile(`^[1-9][0-9]*:[A-Za-z0-9_-]{30,}$`).MatchString(c.Token) {
		return c, errors.New("TELEGRAM_BOT_TOKEN ausente ou com formato inválido")
	}
	var err error
	c.AllowedUserID, err = strconv.ParseInt(get("ALLOWED_TELEGRAM_USER_ID"), 10, 64)
	if err != nil || c.AllowedUserID <= 0 {
		return c, errors.New("ALLOWED_TELEGRAM_USER_ID deve ser um número inteiro positivo")
	}
	c.DatabasePath = strings.TrimSpace(get("DATABASE_PATH"))
	if c.DatabasePath == "" {
		c.DatabasePath = "./data/finance.db"
	}
	zone := get("TIMEZONE")
	if zone == "" {
		zone = "America/Sao_Paulo"
	}
	c.Location, err = time.LoadLocation(zone)
	if err != nil {
		return c, errors.New("TIMEZONE inválido")
	}
	c.StateTTL = 15 * time.Minute
	if v := get("STATE_TTL"); v != "" {
		c.StateTTL, err = time.ParseDuration(v)
		if err != nil || c.StateTTL < time.Minute || c.StateTTL > time.Hour {
			return c, errors.New("STATE_TTL deve estar entre 1m e 1h")
		}
	}
	for name, target := range map[string]*int{"CREDIT_CLOSING_DAY": &c.Credit.ClosingDay, "CREDIT_DUE_DAY": &c.Credit.DueDay} {
		v := get(name)
		if v != "" {
			*target, err = strconv.Atoi(v)
			if err != nil || *target < 1 || *target > 28 {
				return c, errors.New(name + " deve estar entre 1 e 28")
			}
		}
	}
	if err = c.Credit.Validate(); err != nil {
		return c, err
	}
	return c, nil
}
