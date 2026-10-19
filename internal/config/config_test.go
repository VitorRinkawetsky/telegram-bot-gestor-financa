package config

import (
	"strings"
	"testing"
	"time"
)

func valid() map[string]string {
	return map[string]string{"TELEGRAM_BOT_TOKEN": "123456:" + strings.Repeat("a", 35), "ALLOWED_TELEGRAM_USER_ID": "42"}
}
func TestConfig(t *testing.T) {
	m := valid()
	c, err := Read(func(k string) string { return m[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.AllowedUserID != 42 || c.Location.String() != "America/Sao_Paulo" || c.StateTTL != 15*time.Minute || c.DatabasePath != "./data/finance.db" {
		t.Fatalf("%+v", c)
	}
	m["CREDIT_CLOSING_DAY"] = "20"
	m["CREDIT_DUE_DAY"] = "10"
	m["TIMEZONE"] = "UTC"
	m["STATE_TTL"] = "5m"
	c, err = Read(func(k string) string { return m[k] })
	if err != nil || c.Credit.ClosingDay != 20 || c.Location != time.UTC {
		t.Fatalf("%+v %v", c, err)
	}
}
func TestInvalidConfig(t *testing.T) {
	for _, tc := range []struct{ k, v string }{{"TELEGRAM_BOT_TOKEN", ""}, {"TELEGRAM_BOT_TOKEN", "abc"}, {"ALLOWED_TELEGRAM_USER_ID", ""}, {"ALLOWED_TELEGRAM_USER_ID", "-1"}, {"ALLOWED_TELEGRAM_USER_ID", "username"}, {"TIMEZONE", "invalid/zone"}, {"STATE_TTL", "0s"}, {"STATE_TTL", "2h"}, {"CREDIT_CLOSING_DAY", "20"}, {"CREDIT_DUE_DAY", "10"}, {"CREDIT_CLOSING_DAY", "29"}, {"CREDIT_CLOSING_DAY", "0"}, {"CREDIT_DUE_DAY", "abc"}} {
		t.Run(tc.k+tc.v, func(t *testing.T) {
			m := valid()
			m[tc.k] = tc.v
			if _, err := Read(func(k string) string { return m[k] }); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}
