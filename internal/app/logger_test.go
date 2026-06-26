package app

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		level zapcore.Level
		ok    bool
	}{
		{name: "empty defaults to info", value: "", level: zapcore.InfoLevel, ok: true},
		{name: "debug", value: "debug", level: zapcore.DebugLevel, ok: true},
		{name: "info uppercase", value: "INFO", level: zapcore.InfoLevel, ok: true},
		{name: "warn", value: "warn", level: zapcore.WarnLevel, ok: true},
		{name: "warning alias", value: "warning", level: zapcore.WarnLevel, ok: true},
		{name: "error", value: "error", level: zapcore.ErrorLevel, ok: true},
		{name: "trimmed", value: " debug ", level: zapcore.DebugLevel, ok: true},
		{name: "invalid falls back to info", value: "verbose", level: zapcore.InfoLevel, ok: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			level, ok := ParseLogLevel(tt.value)
			if level != tt.level || ok != tt.ok {
				t.Fatalf("ParseLogLevel(%q) = (%s, %v), want (%s, %v)", tt.value, level, ok, tt.level, tt.ok)
			}
		})
	}
}
