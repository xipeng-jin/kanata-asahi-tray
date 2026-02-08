//go:build !linux

package runner

import (
	"context"

	"github.com/rszyma/kanata-tray/config"
)

func startTrackpadWhileTyping(_ context.Context, _ config.TrackpadWhileTyping) {
}
