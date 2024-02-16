package handler

import (
	"context"
	"net"
)

type ConnHandler interface {
	Handle(ctx context.Context, conn net.Conn)
}
