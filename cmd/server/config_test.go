package main

import (
	"testing"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	// Defaults with DevMode = true
	cfg := Config{
		Port:    8080,
		DBPath:  "data/frequencia.db",
		DevMode: true,
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config in dev mode, got %v", err)
	}

	// DevMode = false without Matrix credentials
	prodCfg := cfg
	prodCfg.DevMode = false
	if err := prodCfg.Validate(); err == nil {
		t.Errorf("expected error for missing Matrix credentials in prod mode")
	}

	// Valid prod config
	prodCfg.HomeserverURL = "https://matrix.dc.ufc.br"
	prodCfg.UserID = "@bot:matrix.dc.ufc.br"
	prodCfg.AccessToken = "secret"
	if err := prodCfg.Validate(); err != nil {
		t.Errorf("expected valid prod config, got %v", err)
	}

	// Invalid port
	invalidPort := cfg
	invalidPort.Port = -1
	if err := invalidPort.Validate(); err == nil {
		t.Errorf("expected error for negative port")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DB_PATH", "custom/path.db")
	t.Setenv("DEV_MODE", "true")
	t.Setenv("MATRIX_HOMESERVER_URL", "https://matrix.org/")

	cfg := LoadConfig()
	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.DBPath != "custom/path.db" {
		t.Errorf("expected custom DB path, got %s", cfg.DBPath)
	}
	if !cfg.DevMode {
		t.Errorf("expected devMode true")
	}
	if cfg.HomeserverURL != "https://matrix.org" {
		t.Errorf("expected trimmed trailing slash, got %s", cfg.HomeserverURL)
	}
}

func TestCreateMatrixGateway(t *testing.T) {
	devCfg := Config{DevMode: true}
	gwDev, err := createMatrixGateway(devCfg)
	if err != nil || gwDev == nil {
		t.Fatalf("expected fake gateway for dev mode, got err: %v", err)
	}

	prodCfg := Config{
		HomeserverURL: "https://matrix.org",
		UserID:        "@bot:matrix.org",
		AccessToken:   "tok",
		DevMode:       false,
	}
	gwProd, err := createMatrixGateway(prodCfg)
	if err != nil || gwProd == nil {
		t.Fatalf("expected mautrix gateway for prod mode, got err: %v", err)
	}
}

func TestConfigEmptyDBPath(t *testing.T) {
	cfg := Config{Port: 8080, DBPath: "   ", DevMode: true}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for empty DBPath")
	}
}
