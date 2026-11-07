package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"frequencia-matrix-integration/internal/web"
)

type Config struct {
	Port          int
	DBPath        string
	HomeserverURL string
	UserID        string
	AccessToken   string
	DevMode       bool

	// AutoJoinServers limits which homeservers the bot accepts invites from.
	// Empty means any server.
	AutoJoinServers []string

	// PINSessionTTL is how long a session opened with a room PIN lasts.
	PINSessionTTL time.Duration
}

func LoadConfig() Config {
	port := 8080
	if val := os.Getenv("PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			port = p
		}
	}

	dbPath := "data/frequencia.db"
	if val := os.Getenv("DB_PATH"); val != "" {
		dbPath = val
	}

	devMode := false
	if val := os.Getenv("DEV_MODE"); val != "" {
		devMode = val == "true" || val == "1"
	}

	return Config{
		Port:          port,
		DBPath:        dbPath,
		HomeserverURL: strings.TrimRight(os.Getenv("MATRIX_HOMESERVER_URL"), "/"),
		UserID:        os.Getenv("MATRIX_USER_ID"),
		AccessToken:   os.Getenv("MATRIX_ACCESS_TOKEN"),
		DevMode:       devMode,

		AutoJoinServers: parseServerList(os.Getenv("MATRIX_AUTOJOIN_SERVERS")),
		PINSessionTTL:   parsePINSessionTTL(os.Getenv("PIN_SESSION_TTL")),
	}
}

// parsePINSessionTTL reads a duration such as "24h" or "90m". An empty,
// invalid or non-positive value falls back to the default, like PORT does.
func parsePINSessionTTL(raw string) time.Duration {
	ttl, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || ttl <= 0 {
		return web.DefaultPINSessionTTL
	}
	return ttl
}

// parseServerList splits a comma-separated list, dropping blanks.
func parseServerList(raw string) []string {
	var servers []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			servers = append(servers, item)
		}
	}
	return servers
}

func (c Config) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d, expected 1-65535", c.Port)
	}
	if strings.TrimSpace(c.DBPath) == "" {
		return errors.New("dbPath cannot be empty")
	}

	if !c.DevMode {
		if strings.TrimSpace(c.HomeserverURL) == "" {
			return errors.New("MATRIX_HOMESERVER_URL is required when DEV_MODE is false")
		}
		if strings.TrimSpace(c.UserID) == "" {
			return errors.New("MATRIX_USER_ID is required when DEV_MODE is false")
		}
		if strings.TrimSpace(c.AccessToken) == "" {
			return errors.New("MATRIX_ACCESS_TOKEN is required when DEV_MODE is false")
		}
	}

	return nil
}
