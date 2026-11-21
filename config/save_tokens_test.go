package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/model"
)

func TestSaveTokensWarnsOnWriteFailure(t *testing.T) {
	path := t.TempDir()
	var warnings bytes.Buffer
	previous := log.Warning.Writer()
	log.Warning.SetOutput(&warnings)
	t.Cleanup(func() { log.Warning.SetOutput(previous) })

	const token = "synthetic-device-token"
	SaveTokens(path, model.AuthTokens{DeviceToken: token})

	message := warnings.String()
	if !strings.Contains(message, "failed to save config") || !strings.Contains(message, path) {
		t.Fatalf("expected a warning about the failed write to %q, got %q", path, message)
	}
	if strings.Contains(message, token) {
		t.Fatal("save warning contains the device token")
	}
}
