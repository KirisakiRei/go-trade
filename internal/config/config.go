package config

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	DBPath             string
	DefaultSymbol      string
	ContextTF          string
	EntryTF            string
	MonitorIntervalSec int
	AllowedOrigins     string
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Info("No .env file found, using system environment variables")
	}

	interval, _ := strconv.Atoi(getEnv("MONITOR_INTERVAL_SEC", "30"))
	if interval <= 0 {
		interval = 30
	}

	return &Config{
		Port:               getEnv("PORT", "8080"),
		DBPath:             getEnv("DB_PATH", "ila.db"),
		DefaultSymbol:      getEnv("DEFAULT_SYMBOL", "XAUUSD"),
		ContextTF:          getEnv("CONTEXT_TF", "H1"),
		EntryTF:            getEnv("ENTRY_TF", "M15"),
		MonitorIntervalSec: interval,
		AllowedOrigins:     getEnv("ALLOWED_ORIGINS", "*"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
