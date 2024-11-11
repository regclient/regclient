package sloghandle

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/regclient/regclient/types"
)

func TestLogrus(t *testing.T) {
	tt := []struct {
		name         string
		logrusLevel  logrus.Level
		slogLevel    slog.Level
		inexactLevel bool
	}{
		{
			name:        "trace",
			logrusLevel: logrus.TraceLevel,
			slogLevel:   types.LevelTrace,
		},
		{
			name:        "debug",
			logrusLevel: logrus.DebugLevel,
			slogLevel:   slog.LevelDebug,
		},
		{
			name:        "info",
			logrusLevel: logrus.InfoLevel,
			slogLevel:   slog.LevelInfo,
		},
		{
			name:        "warn",
			logrusLevel: logrus.WarnLevel,
			slogLevel:   slog.LevelWarn,
		},
		{
			name:        "fatal",
			logrusLevel: logrus.FatalLevel,
			slogLevel:   slog.LevelError + 4,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			// compare level mapping
			if slogToLogrus(tc.slogLevel) != tc.logrusLevel {
				t.Errorf("convert to logrus level expected %v, received %v", tc.logrusLevel, slogToLogrus(tc.slogLevel))
			}
			if !tc.inexactLevel && logrusToSlog[tc.logrusLevel] != tc.slogLevel {
				t.Errorf("convert to slog level expected %v, received %v", tc.slogLevel, logrusToSlog[tc.logrusLevel])
			}
			// create an slog handler with the default text logger
			out := &bytes.Buffer{}
			logrusLogger := logrus.New()
			logrusLogger.Out = out
			logrusLogger.Level = tc.logrusLevel
			slogLogger := slog.New(Logrus(logrusLogger))
			// generate some sample logs
			slogLogger.Debug("test debug message", "attr1", "value1", "attr2", 2)
			slogLogger.Warn("test warn message inline", "attr3", "value3", "attr4", 4)
			// create a child logger
			slogChild := slog.New(slogLogger.Handler().WithGroup("child").WithAttrs([]slog.Attr{slog.String("child", "value")}))
			slogChild.Info("test info message", "attr5", "value5", "attr6", 6)
			// add a few more tests
			slogLogger.Warn("test warn with formatted attributes",
				slog.Group("child2", slog.String("attr-c1", "c1"), slog.Int("attr-c2", 2)),
				slog.String("attr7", "value7"), slog.Int("attr8", 8))
			slogLogger.Log(ctx, types.LevelTrace, "test trace message", "attr9", "value9")
			// check output for logs and check if enabled based on logging level
			logs := out.String()
			t.Logf("all logs:\n%s", logs)
			if strings.Contains(logs, "test trace message") {
				if tc.slogLevel > types.LevelTrace {
					t.Errorf("trace message seen")
				}
			} else if tc.slogLevel <= types.LevelTrace {
				t.Errorf("trace message not seen")
			}
			if strings.Contains(logs, "test debug message") {
				if tc.slogLevel > slog.LevelDebug {
					t.Errorf("debug message seen")
				}
			} else if tc.slogLevel <= slog.LevelDebug {
				t.Errorf("debug message not seen")
			}
			if strings.Contains(logs, "test info message") {
				if !strings.Contains(logs, "child:child") {
					t.Errorf("child:child not seen in info message")
				}
				if tc.slogLevel > slog.LevelInfo {
					t.Errorf("info message seen")
				}
			} else if tc.slogLevel <= slog.LevelInfo {
				t.Errorf("info message not seen")
			}
			if strings.Contains(logs, "test warn message inline") {
				if tc.slogLevel > slog.LevelWarn {
					t.Errorf("warn message (inline) seen")
				}
			} else if tc.slogLevel <= slog.LevelWarn {
				t.Errorf("warn message (inline) not seen")
			}
			if strings.Contains(logs, "test warn with formatted attributes") {
				if !strings.Contains(logs, "child2") {
					t.Errorf("child2 not seen in warn message")
				}
				if tc.slogLevel > slog.LevelWarn {
					t.Errorf("warn message (formatted attributes) seen")
				}
			} else if tc.slogLevel <= slog.LevelWarn {
				t.Errorf("warn message (formatted attributes) not seen")
			}
		})
	}
}
