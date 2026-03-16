//go:build linux

package runner

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/gommon/log"

	"github.com/rszyma/kanata-tray/config"
)

const (
	evKey            uint16 = 0x01
	keyCodeLeftAlt   uint16 = 56
	keyCodeRightAlt  uint16 = 100
	keyCodeLeftMeta  uint16 = 125
	keyCodeRightMeta uint16 = 126

	keyboardDeviceReadyTimeout  = 8 * time.Second
	keyboardDeviceRetryInterval = 100 * time.Millisecond
)

type trackpadControlState struct {
	pointerEnabled                       bool
	permanentEnabled                     bool
	comboPressed                         bool
	triggerPressed                       bool
	suppressTemporaryUntilTriggerRelease bool
	leftAltPressed                       bool
	rightAltPressed                      bool
	leftMetaPressed                      bool
	rightMetaPressed                     bool
}

func startTrackpadWhileTyping(ctx context.Context, cfg config.TrackpadWhileTyping) {
	if !cfg.Enabled {
		return
	}

	go func() {
		err := runTrackpadWhileTyping(ctx, cfg)
		if err != nil && ctx.Err() == nil {
			log.Warnf("trackpad_while_typing failed: %v", err)
		}
	}()
}

func runTrackpadWhileTyping(ctx context.Context, cfg config.TrackpadWhileTyping) error {
	pointerDeviceName, err := resolvePointerDeviceName(ctx, cfg.PointerDevice)
	if err != nil {
		return fmt.Errorf("resolve pointer device: %w", err)
	}

	if err := setHyprPointerEnabled(ctx, pointerDeviceName, false); err != nil {
		return fmt.Errorf("disable pointer '%s': %w", pointerDeviceName, err)
	}
	state := newTrackpadControlState()

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := setHyprPointerEnabled(cleanupCtx, pointerDeviceName, true); err != nil {
			log.Warnf("trackpad_while_typing cleanup failed to re-enable pointer '%s': %v", pointerDeviceName, err)
		}
	}()

	keyboardDeviceFile, keyboardEventPath, err := openKeyboardDeviceWithRetry(ctx, cfg.KeyboardDevice)
	if err != nil {
		return err
	}
	defer keyboardDeviceFile.Close()

	go func() {
		<-ctx.Done()
		_ = keyboardDeviceFile.Close()
	}()

	log.Infof(
		"trackpad_while_typing enabled (keyboard=%s pointer=%s trigger=KEY_ALT toggle_combo=KEY_ALT+KEY_META)",
		keyboardEventPath,
		pointerDeviceName,
	)

	for {
		var event inputEvent
		err := binary.Read(keyboardDeviceFile, binary.NativeEndian, &event)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read keyboard events: %w", err)
		}

		if event.Type != evKey || !isTrackpadControlKey(event.Code) {
			continue
		}

		pressed, isStateEvent := keyValueToPressedState(event.Value)
		if !isStateEvent {
			continue
		}

		pointerChanged, permanentChanged := state.applyKeyEvent(event.Code, pressed)
		if permanentChanged {
			log.Infof("trackpad_while_typing permanent toggle changed: enabled=%t", state.permanentEnabled)
		}
		if !pointerChanged {
			continue
		}

		err = setHyprPointerEnabled(ctx, pointerDeviceName, state.pointerEnabled)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("toggle pointer enabled=%t: %w", state.pointerEnabled, err)
		}
	}
}

func resolvePointerDeviceName(ctx context.Context, rawValue string) (string, error) {
	if rawValue == "" {
		rawValue = "auto:touchpad"
	}

	rawValue = strings.TrimSpace(rawValue)
	if rawValue == "auto:touchpad" {
		mice, err := hyprMouseDeviceNames(ctx)
		if err != nil {
			return "", err
		}

		for _, mouseName := range mice {
			nameLower := strings.ToLower(mouseName)
			if strings.Contains(nameLower, "trackpad") || strings.Contains(nameLower, "touchpad") {
				return mouseName, nil
			}
		}

		for _, mouseName := range mice {
			if !strings.Contains(strings.ToLower(mouseName), "kanata") {
				return mouseName, nil
			}
		}

		return "", fmt.Errorf("could not auto-detect pointer device in hyprctl output")
	}

	if strings.HasPrefix(rawValue, "auto:") {
		return "", fmt.Errorf("unsupported pointer auto selector '%s'", rawValue)
	}

	return rawValue, nil
}

func resolveKeyboardEventPath(rawValue string) (string, error) {
	if rawValue == "" {
		rawValue = "auto:kanata"
	}

	rawValue = strings.TrimSpace(rawValue)
	if rawValue == "auto:kanata" {
		return autoDetectKanataEventDevice()
	}

	if strings.HasPrefix(rawValue, "auto:") {
		return "", fmt.Errorf("unsupported keyboard auto selector '%s'", rawValue)
	}

	return rawValue, nil
}

func openKeyboardDeviceWithRetry(ctx context.Context, rawValue string) (*os.File, string, error) {
	deadline := time.Now().Add(keyboardDeviceReadyTimeout)
	autoSelector := isAutoKanataSelector(rawValue)

	var lastErr error
	for {
		keyboardEventPath, err := resolveKeyboardEventPath(rawValue)
		if err != nil {
			lastErr = fmt.Errorf("resolve keyboard device: %w", err)
			if !autoSelector {
				return nil, "", lastErr
			}
		} else {
			keyboardDeviceFile, openErr := os.Open(keyboardEventPath)
			if openErr == nil {
				return keyboardDeviceFile, keyboardEventPath, nil
			}

			lastErr = fmt.Errorf("open keyboard device '%s': %w", keyboardEventPath, openErr)
			if !isRetriableKeyboardOpenError(openErr) {
				return nil, "", lastErr
			}
		}

		if !autoSelector && time.Now().After(deadline) {
			return nil, "", lastErr
		}
		if autoSelector && time.Now().After(deadline) {
			return nil, "", fmt.Errorf("timed out waiting for keyboard device readiness: %w", lastErr)
		}

		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(keyboardDeviceRetryInterval):
		}
	}
}

func isAutoKanataSelector(rawValue string) bool {
	rawValue = strings.TrimSpace(rawValue)
	return rawValue == "" || strings.EqualFold(rawValue, "auto:kanata")
}

func isRetriableKeyboardOpenError(err error) bool {
	if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) {
		return true
	}

	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		return false
	}

	errno, ok := pathErr.Err.(syscall.Errno)
	if !ok {
		return false
	}

	switch errno {
	case syscall.EACCES, syscall.EPERM, syscall.ENOENT, syscall.ENODEV, syscall.ENXIO, syscall.EBUSY:
		return true
	default:
		return false
	}
}

func autoDetectKanataEventDevice() (string, error) {
	eventPaths, err := filepath.Glob("/sys/class/input/event*")
	if err != nil {
		return "", fmt.Errorf("scan /sys/class/input: %w", err)
	}

	for _, eventPath := range eventPaths {
		namePath := filepath.Join(eventPath, "device/name")
		nameData, err := os.ReadFile(namePath)
		if err != nil {
			continue
		}

		if !strings.Contains(strings.ToLower(strings.TrimSpace(string(nameData))), "kanata") {
			continue
		}

		eventNode := filepath.Base(eventPath)
		devPath := filepath.Join("/dev/input", eventNode)
		if _, err := os.Stat(devPath); err == nil {
			return devPath, nil
		}
	}

	return "", fmt.Errorf("auto:kanata keyboard device not found")
}

func isTrackpadControlKey(code uint16) bool {
	return code == keyCodeLeftAlt ||
		code == keyCodeRightAlt ||
		code == keyCodeLeftMeta ||
		code == keyCodeRightMeta
}

func keyValueToPressedState(value int32) (bool, bool) {
	switch value {
	case 0:
		return false, true
	case 1, 2:
		return true, true
	default:
		return false, false
	}
}

func newTrackpadControlState() trackpadControlState {
	return trackpadControlState{pointerEnabled: false}
}

func (s *trackpadControlState) applyKeyEvent(code uint16, pressed bool) (bool, bool) {
	previousPointerEnabled := s.pointerEnabled
	previousPermanentEnabled := s.permanentEnabled

	switch code {
	case keyCodeLeftAlt:
		s.leftAltPressed = pressed
	case keyCodeRightAlt:
		s.rightAltPressed = pressed
	case keyCodeLeftMeta:
		s.leftMetaPressed = pressed
	case keyCodeRightMeta:
		s.rightMetaPressed = pressed
	}
	s.triggerPressed = s.leftAltPressed || s.rightAltPressed

	comboNow := s.triggerPressed && (s.leftMetaPressed || s.rightMetaPressed)
	if comboNow && !s.comboPressed {
		s.permanentEnabled = !s.permanentEnabled
		if !s.permanentEnabled {
			s.suppressTemporaryUntilTriggerRelease = s.triggerPressed
		}
	}
	s.comboPressed = comboNow
	if !s.triggerPressed {
		s.suppressTemporaryUntilTriggerRelease = false
	}

	s.pointerEnabled = s.permanentEnabled
	if !s.pointerEnabled {
		s.pointerEnabled = s.triggerPressed && !s.comboPressed && !s.suppressTemporaryUntilTriggerRelease
	}

	return s.pointerEnabled != previousPointerEnabled, s.permanentEnabled != previousPermanentEnabled
}

func setHyprPointerEnabled(ctx context.Context, pointerDeviceName string, enabled bool) error {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	state := strconv.FormatBool(enabled)
	keyword := fmt.Sprintf("device[%s]:enabled", pointerDeviceName)

	stdout, stderr, err := runCommand(ctxWithTimeout, "hyprctl", "-r", "--", "keyword", keyword, state)
	if err != nil {
		return fmt.Errorf(
			"hyprctl keyword %s %s failed: %w (stdout=%q stderr=%q)",
			keyword,
			state,
			err,
			strings.TrimSpace(string(stdout)),
			strings.TrimSpace(string(stderr)),
		)
	}

	return nil
}

func hyprMouseDeviceNames(ctx context.Context) ([]string, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	stdout, stderr, err := runCommand(ctxWithTimeout, "hyprctl", "-j", "devices")
	if err != nil {
		return nil, fmt.Errorf(
			"hyprctl -j devices failed: %w (stdout=%q stderr=%q)",
			err,
			strings.TrimSpace(string(stdout)),
			strings.TrimSpace(string(stderr)),
		)
	}

	var payload struct {
		Mice []struct {
			Name string `json:"name"`
		} `json:"mice"`
	}

	err = json.Unmarshal(stdout, &payload)
	if err != nil {
		return nil, fmt.Errorf("parse hyprctl -j devices output: %w", err)
	}
	if len(payload.Mice) == 0 {
		return nil, fmt.Errorf("hyprctl -j devices returned no mice")
	}

	result := make([]string, 0, len(payload.Mice))
	for _, mouse := range payload.Mice {
		name := strings.TrimSpace(mouse.Name)
		if name != "" {
			result = append(result, name)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("hyprctl -j devices did not include usable mouse names")
	}

	return result, nil
}

func runCommand(ctx context.Context, binaryName string, args ...string) ([]byte, []byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	command := cmd(ctx, &stdout, &stderr, binaryName, args...)
	err := command.Run()

	return stdout.Bytes(), stderr.Bytes(), err
}

type inputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}
