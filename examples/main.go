package main

import (
	"context"

	"github.com/squaredcow/yats/pkg/config"
	"github.com/squaredcow/yats/pkg/tcpserver"
)

func main() {

	logger := config.NewLogger()
	defer config.CloseLogger(logger)

	cfg := config.TcpServerConfigs{
		Type: "tcp",
		Host: "localhost",
		Port: "3000",
		Pool: config.TCPServerPoolConfigs{
			MaxSize:            3,
			WaitForConnTimeout: 5000,
			NoOpCycle:          3000,
		},
	}

	handler := NewDefaultHandler(logger)

	server := tcpserver.NewTcpServer(logger, cfg, handler)

	ctx := context.Background()
	server.Start(ctx)
}
