package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var l *zap.SugaredLogger

func Logger() *zap.SugaredLogger {
	return l
}

func init() {
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})

	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.ErrorLevel && lvl > zapcore.DebugLevel
	})

	if d := os.Getenv("BLANK_DEBUG"); len(d) > 0 && d != "false" {
		lowPriority = zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl < zapcore.ErrorLevel
		})
	}

	// if j := os.Getenv("BLANK_JSON_LOGGING"); len(j) > 0 {
	// 	log.SetFormatter(&log.JSONFormatter{})
	// }

	consoleDebugging := zapcore.Lock(os.Stdout)
	consoleErrors := zapcore.Lock(os.Stderr)
	consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, consoleErrors, highPriority),
		zapcore.NewCore(consoleEncoder, consoleDebugging, lowPriority),
	)

	logger := zap.New(core)
	defer logger.Sync()

	l = logger.Sugar()
}
