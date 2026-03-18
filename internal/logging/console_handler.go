package logging

import (
	"io"
	"log/slog"
	"strings"
	"time"
)

func newConsoleHandler(writer io.Writer, level slog.Level) slog.Handler {
	return slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				attr.Key = "ts"
				if ts, ok := attr.Value.Any().(time.Time); ok {
					attr.Value = slog.StringValue(ts.Format(time.RFC3339))
				}
			case slog.LevelKey:
				attr.Value = slog.StringValue(strings.ToUpper(attr.Value.String()))
			}
			return attr
		},
	})
}
