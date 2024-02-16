package tcpserver

import (
	"context"
)

const ctxTraceID = 0

type Server interface {
	Start(ctx context.Context)
	Close()
}
