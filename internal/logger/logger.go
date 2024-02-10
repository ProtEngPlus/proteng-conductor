package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
)

var Zap *zap.Logger

func InitZap() {
	if os.Getenv("ENV") == "prod" {
		_logger, _ := zap.NewProduction()
		Zap = _logger
	} else {
		_logger, _ := zap.NewDevelopment()
		Zap = _logger
	}

	Zap.Info("Logger initialized")
}

func CloseZap() {
	_ = Zap.Sync()
}

// Wrapper for printf style logging

func Infof(format string, v ...any) {
	Zap.Info(fmt.Sprintf(format, v...))
}

func Errorf(format string, v ...any) {
	Zap.Error(fmt.Sprintf(format, v...))
}

func Debugf(format string, v ...any) {
	Zap.Debug(fmt.Sprintf(format, v...))
}

func Warnf(format string, v ...any) {
	Zap.Warn(fmt.Sprintf(format, v...))
}

func Fatalf(format string, v ...any) {
	Zap.Fatal(fmt.Sprintf(format, v...))
}
