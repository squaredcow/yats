package main

import (
	"context"
	"net"
	"time"

	"go.uber.org/zap"
)

type DefaultHandler struct {
	log *zap.Logger
}

func NewDefaultHandler(log *zap.Logger) *DefaultHandler {
	return &DefaultHandler{log: log}
}

func (h *DefaultHandler) Handle(ctx context.Context, conn net.Conn) {
	traceLog := h.log.With(zap.Any("traceId", ctx.Value(0)))
	traceLog.Info("default_handler: start handling packages")
	time.Sleep(time.Second * 3)
	traceLog.Info("default_handler: finish handling packages")
}
