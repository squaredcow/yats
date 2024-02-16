package tcpserver

import (
	"context"
)

type Server interface {
	Start(ctx context.Context)
	Close()
}
