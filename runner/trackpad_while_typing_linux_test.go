//go:build linux

package runner

import "testing"

func TestTrackpadControlStateTemporaryEnableWithAlt(t *testing.T) {
	state := newTrackpadControlState()

	pointerChanged, permanentChanged := state.applyKeyEvent(keyCodeLeftAlt, true)
	if !pointerChanged || permanentChanged {
		t.Fatalf("alt press should enable pointer temporarily without permanent toggle, got pointerChanged=%t permanentChanged=%t", pointerChanged, permanentChanged)
	}
	if !state.pointerEnabled {
		t.Fatal("pointer should be enabled while alt trigger is pressed")
	}

	pointerChanged, permanentChanged = state.applyKeyEvent(keyCodeLeftAlt, false)
	if !pointerChanged || permanentChanged {
		t.Fatalf("alt release should disable pointer without permanent toggle, got pointerChanged=%t permanentChanged=%t", pointerChanged, permanentChanged)
	}
	if state.pointerEnabled {
		t.Fatal("pointer should be disabled after alt trigger release")
	}
}

func TestTrackpadControlStateAltMetaTogglePersistsAfterRelease(t *testing.T) {
	state := newTrackpadControlState()

	state.applyKeyEvent(keyCodeLeftAlt, true)
	pointerChanged, permanentChanged := state.applyKeyEvent(keyCodeLeftMeta, true)
	if !permanentChanged {
		t.Fatal("alt+leftmeta press should toggle permanent mode on")
	}
	if pointerChanged {
		t.Fatal("pointer should already be enabled while alt is held")
	}
	if !state.permanentEnabled || !state.pointerEnabled {
		t.Fatal("pointer should remain enabled in permanent mode")
	}

	state.applyKeyEvent(keyCodeLeftMeta, false)
	state.applyKeyEvent(keyCodeLeftAlt, false)
	if !state.permanentEnabled || !state.pointerEnabled {
		t.Fatal("permanent mode should keep pointer enabled after combo release")
	}
}

func TestTrackpadControlStateRightAltMetaTogglePersistsAfterRelease(t *testing.T) {
	state := newTrackpadControlState()

	state.applyKeyEvent(keyCodeRightAlt, true)
	pointerChanged, permanentChanged := state.applyKeyEvent(keyCodeRightMeta, true)
	if !permanentChanged {
		t.Fatal("rightalt+rightmeta press should toggle permanent mode on")
	}
	if pointerChanged {
		t.Fatal("pointer should already be enabled while alt is held")
	}
	if !state.permanentEnabled || !state.pointerEnabled {
		t.Fatal("pointer should remain enabled in permanent mode")
	}

	state.applyKeyEvent(keyCodeRightMeta, false)
	state.applyKeyEvent(keyCodeRightAlt, false)
	if !state.permanentEnabled || !state.pointerEnabled {
		t.Fatal("permanent mode should keep pointer enabled after meta combo release")
	}
}

func TestTrackpadControlStateToggleOffSuppressesTemporaryUntilAltRelease(t *testing.T) {
	state := newTrackpadControlState()

	state.applyKeyEvent(keyCodeLeftAlt, true)
	state.applyKeyEvent(keyCodeLeftMeta, true)
	state.applyKeyEvent(keyCodeLeftMeta, false)

	pointerChanged, permanentChanged := state.applyKeyEvent(keyCodeLeftMeta, true)
	if !permanentChanged {
		t.Fatal("second alt+leftmeta press should toggle permanent mode off")
	}
	if !pointerChanged {
		t.Fatal("toggling permanent mode off while alt is held should disable the pointer")
	}
	if state.pointerEnabled {
		t.Fatal("pointer should be disabled after permanent mode is toggled off")
	}

	pointerChanged, permanentChanged = state.applyKeyEvent(keyCodeLeftMeta, false)
	if pointerChanged || permanentChanged {
		t.Fatalf("releasing meta while alt stays held should not re-enable temporary mode, got pointerChanged=%t permanentChanged=%t", pointerChanged, permanentChanged)
	}
	if state.pointerEnabled {
		t.Fatal("pointer should remain disabled until alt trigger is released")
	}

	state.applyKeyEvent(keyCodeLeftAlt, false)
	if state.pointerEnabled {
		t.Fatal("pointer should remain disabled after alt release when permanent mode is off")
	}
}

func TestTrackpadControlKeyIncludesAltAndMetaKeys(t *testing.T) {
	if !isTrackpadControlKey(keyCodeLeftAlt) {
		t.Fatal("left alt should be treated as a valid trigger key")
	}
	if !isTrackpadControlKey(keyCodeLeftMeta) {
		t.Fatal("left meta should be treated as a valid combo partner")
	}
}
