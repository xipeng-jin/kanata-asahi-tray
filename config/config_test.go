package config

import "testing"

func TestTrackpadWhileTypingDefaultTriggerKeyIsEsc(t *testing.T) {
	cfg := defaultTrackpadWhileTyping()

	if cfg.TriggerKey != "KEY_ESC" {
		t.Fatalf("default trigger key = %q, want KEY_ESC", cfg.TriggerKey)
	}
}

func TestTrackpadWhileTypingTriggerKeyEscapeAlias(t *testing.T) {
	raw := "key_escape"
	cfg, err := (&trackpadWhileTyping{TriggerKey: &raw}).intoExported()
	if err != nil {
		t.Fatalf("KEY_ESCAPE alias should be accepted: %v", err)
	}
	if cfg.TriggerKey != "KEY_ESC" {
		t.Fatalf("trigger key = %q, want KEY_ESC", cfg.TriggerKey)
	}
}

func TestTrackpadWhileTypingRejectsOldAltTriggerKey(t *testing.T) {
	raw := "KEY_ALT"
	_, err := (&trackpadWhileTyping{TriggerKey: &raw}).intoExported()
	if err == nil {
		t.Fatal("KEY_ALT should be rejected")
	}
}
