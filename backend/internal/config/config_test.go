package config

import "testing"

func TestParseDefaults(t *testing.T) {
	cfg, err := parse([]byte(`{"password":"x"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want default 9090", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want default 127.0.0.1", cfg.Host)
	}
	// ServeLanding is cloud-only: a local self-hosted wede must default to off so
	// the marketing landing never shadows the IDE's own /assets/*.
	if cfg.ServeLanding {
		t.Error("ServeLanding should default to false")
	}
}

func TestParseServeLanding(t *testing.T) {
	cfg, err := parse([]byte(`{"password":"x","serve_landing":true}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.ServeLanding {
		t.Error("serve_landing:true should set ServeLanding")
	}
}

func TestParseRequiresPassword(t *testing.T) {
	if _, err := parse([]byte(`{"port":"1234"}`)); err == nil {
		t.Fatal("expected error when password is missing")
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	if _, err := parse([]byte(`{"password":"x","bogus":1}`)); err == nil {
		t.Fatal("expected error for unknown config key")
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, err := parse([]byte(`{not json`)); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseOverridesDefaults(t *testing.T) {
	cfg, err := parse([]byte(`{"password":"x","port":"3000","host":"0.0.0.0"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Port != "3000" || cfg.Host != "0.0.0.0" {
		t.Errorf("overrides not applied: port=%q host=%q", cfg.Port, cfg.Host)
	}
}
