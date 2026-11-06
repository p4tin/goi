package rpc

import (
	"log/slog"
	"os"
)

// wailsLogger routes Wails' internal logging into the Goi log.
type wailsLogger struct {
	log *slog.Logger
}

func (l wailsLogger) Print(m string)   { l.log.Info(m, "src", "wails") }
func (l wailsLogger) Trace(m string)   { l.log.Debug(m, "src", "wails") }
func (l wailsLogger) Debug(m string)   { l.log.Debug(m, "src", "wails") }
func (l wailsLogger) Info(m string)    { l.log.Info(m, "src", "wails") }
func (l wailsLogger) Warning(m string) { l.log.Warn(m, "src", "wails") }
func (l wailsLogger) Error(m string)   { l.log.Error(m, "src", "wails") }
func (l wailsLogger) Fatal(m string) {
	l.log.Error(m, "src", "wails", "fatal", true)
	os.Exit(1)
}
