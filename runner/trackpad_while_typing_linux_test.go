//go:build linux

package runner

import "testing"

func TestTrackpadControlStateStartsEnabledWhenOmarchyFlagIsAbsent(t *testing.T) {
	state := newTrackpadControlState(false)

	if !state.pointerEnabled {
		t.Fatal("pointer should start enabled when Omarchy touchpad-disabled flag is absent")
	}
}

func TestTrackpadControlStateStartsDisabledWhenOmarchyFlagExists(t *testing.T) {
	state := newTrackpadControlState(true)

	if state.pointerEnabled {
		t.Fatal("pointer should start disabled when Omarchy touchpad-disabled flag exists")
	}
}

func TestTrackpadControlStateTemporaryEnableWithEscWhenOmarchyDisabled(t *testing.T) {
	state := newTrackpadControlState(true)

	pointerChanged := state.applyKeyEvent(keyCodeEsc, true, true)
	if !pointerChanged {
		t.Fatal("esc press should enable pointer temporarily when Omarchy disabled flag exists")
	}
	if !state.pointerEnabled || !state.temporaryEnabled {
		t.Fatal("pointer should be temporarily enabled while esc trigger is pressed")
	}

	pointerChanged = state.applyKeyEvent(keyCodeEsc, false, true)
	if !pointerChanged {
		t.Fatal("esc release should disable pointer again when Omarchy disabled flag still exists")
	}
	if state.pointerEnabled || state.temporaryEnabled {
		t.Fatal("pointer should be disabled after esc trigger release")
	}
}

func TestTrackpadControlStateEscDoesNothingWhenOmarchyEnabled(t *testing.T) {
	state := newTrackpadControlState(false)

	pointerChanged := state.applyKeyEvent(keyCodeEsc, true, false)
	if pointerChanged {
		t.Fatal("esc press should not change pointer when Omarchy disabled flag is absent")
	}
	if !state.pointerEnabled || state.temporaryEnabled {
		t.Fatal("pointer should remain normally enabled")
	}

	pointerChanged = state.applyKeyEvent(keyCodeEsc, false, false)
	if pointerChanged {
		t.Fatal("esc release should not change pointer when Omarchy disabled flag is absent")
	}
	if !state.pointerEnabled || state.temporaryEnabled {
		t.Fatal("pointer should remain normally enabled after esc release")
	}
}

func TestTrackpadControlStateFlagRemovedBeforeEscReleaseKeepsPointerEnabled(t *testing.T) {
	state := newTrackpadControlState(true)

	if !state.applyKeyEvent(keyCodeEsc, true, true) {
		t.Fatal("esc press should temporarily enable pointer")
	}
	if state.applyKeyEvent(keyCodeEsc, false, false) {
		t.Fatal("esc release should not change pointer when Omarchy flag was removed")
	}
	if !state.pointerEnabled || state.temporaryEnabled {
		t.Fatal("pointer should remain normally enabled after Omarchy flag is removed")
	}
}

func TestTrackpadControlStateIgnoresAltKeys(t *testing.T) {
	state := newTrackpadControlState(true)

	if state.applyKeyEvent(keyCodeLeftAlt, true, true) {
		t.Fatal("left alt should not change pointer")
	}
	if state.applyKeyEvent(keyCodeRightAlt, true, true) {
		t.Fatal("right alt should not change pointer")
	}
	if state.pointerEnabled {
		t.Fatal("pointer should remain disabled after ignored alt keys")
	}
}

func TestTrackpadControlKeyIncludesOnlyTriggerKey(t *testing.T) {
	if !isTrackpadControlKey(keyCodeEsc, keyCodeEsc) {
		t.Fatal("esc should be treated as a valid trigger key")
	}
	if isTrackpadControlKey(keyCodeLeftAlt, keyCodeEsc) {
		t.Fatal("left alt should not be treated as a trackpad control key")
	}
}

func TestTriggerKeyCodeSupportsEscAliases(t *testing.T) {
	code, err := triggerKeyCode("KEY_ESC")
	if err != nil || code != keyCodeEsc {
		t.Fatalf("KEY_ESC should resolve to esc code, got code=%d err=%v", code, err)
	}

	code, err = triggerKeyCode("key_escape")
	if err != nil || code != keyCodeEsc {
		t.Fatalf("KEY_ESCAPE should resolve to esc code, got code=%d err=%v", code, err)
	}

	if _, err = triggerKeyCode("KEY_ALT"); err == nil {
		t.Fatal("old KEY_ALT trigger should be rejected")
	}
}
