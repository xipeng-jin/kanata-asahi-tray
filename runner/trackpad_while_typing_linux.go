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
	evKey           uint16 = 0x01
	keyCodeLeftAlt  uint16 = 56
	keyCodeRightAlt uint16 = 100
	keyCodeFn       uint16 = 0x1d0

	keyboardDeviceReadyTimeout  = 8 * time.Second
	keyboardDeviceRetryInterval = 100 * time.Millisecond
)

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

	triggerCode, err := triggerKeyCode(cfg.TriggerKey)
	if err != nil {
		return fmt.Errorf("parse trigger key: %w", err)
	}

	if err := setHyprPointerEnabled(ctx, pointerDeviceName, false); err != nil {
		return fmt.Errorf("disable pointer '%s': %w", pointerDeviceName, err)
	}
	pointerEnabled := false
	permanentEnabled := false
	comboPressed := false
	triggerPressed := false
	suppressTemporaryUntilTriggerRelease := false
	fnPressed := false
	leftAltPressed := false
	rightAltPressed := false

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
		"trackpad_while_typing enabled (keyboard=%s pointer=%s trigger=%s toggle_combo=KEY_FN+KEY_LEFTALT|KEY_RIGHTALT)",
		keyboardEventPath,
		pointerDeviceName,
		cfg.TriggerKey,
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

		if event.Type != evKey || !isTrackpadControlKey(event.Code, triggerCode) {
			continue
		}

		pressed, isStateEvent := keyValueToPressedState(event.Value)
		if !isStateEvent {
			continue
		}

		switch event.Code {
		case keyCodeFn:
			fnPressed = pressed
		case keyCodeLeftAlt:
			leftAltPressed = pressed
		case keyCodeRightAlt:
			rightAltPressed = pressed
		}
		if event.Code == triggerCode {
			triggerPressed = pressed
		}

		comboNow := fnPressed && (leftAltPressed || rightAltPressed)
		if comboNow && !comboPressed {
			permanentEnabled = !permanentEnabled
			if !permanentEnabled {
				suppressTemporaryUntilTriggerRelease = triggerPressed
			}
			log.Infof("trackpad_while_typing permanent toggle changed: enabled=%t", permanentEnabled)
		}
		comboPressed = comboNow
		if !triggerPressed {
			suppressTemporaryUntilTriggerRelease = false
		}

		shouldEnablePointer := permanentEnabled
		if !shouldEnablePointer {
			shouldEnablePointer = triggerPressed && !comboPressed && !suppressTemporaryUntilTriggerRelease
		}

		if shouldEnablePointer == pointerEnabled {
			continue
		}

		err = setHyprPointerEnabled(ctx, pointerDeviceName, shouldEnablePointer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("toggle pointer enabled=%t: %w", shouldEnablePointer, err)
		}

		pointerEnabled = shouldEnablePointer
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

func triggerKeyCode(triggerKey string) (uint16, error) {
	switch strings.ToUpper(strings.TrimSpace(triggerKey)) {
	case "", "KEY_FN":
		return keyCodeFn, nil
	case "KEY_LEFTALT":
		return keyCodeLeftAlt, nil
	case "KEY_RIGHTALT":
		return keyCodeRightAlt, nil
	default:
		return 0, fmt.Errorf("unsupported trigger key '%s'", triggerKey)
	}
}

func isTrackpadControlKey(code uint16, triggerCode uint16) bool {
	return code == triggerCode || code == keyCodeFn || code == keyCodeLeftAlt || code == keyCodeRightAlt
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
