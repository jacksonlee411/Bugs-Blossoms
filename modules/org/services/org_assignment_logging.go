package services

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/pkg/constants"
)

func loggerFromContext(ctx context.Context) *logrus.Entry {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(constants.LoggerKey)
	switch typed := v.(type) {
	case *logrus.Entry:
		return typed
	case *logrus.Logger:
		return logrus.NewEntry(typed)
	default:
		return nil
	}
}

func logWithFields(ctx context.Context, level logrus.Level, msg string, fields logrus.Fields) {
	logger := loggerFromContext(ctx)
	if logger == nil {
		return
	}
	logger.WithFields(fields).Log(level, msg)
}
