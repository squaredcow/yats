package main

import (
	"context"

	"github.com/squaredcow/yats/pkg/config"
	"github.com/squaredcow/yats/pkg/tcpserver"
	"go.uber.org/zap"
)

func main() {

	logger, _ := zap.NewDevelopment()

	// encoderConfig := ecszap.NewDefaultEncoderConfig()
	// core := ecszap.NewCore(encoderConfig, os.Stdout, zap.InfoLevel)
	// logger := config.NewLogger(core)

	defer config.CloseLogger(logger)

	cfg := config.TcpServerConfigs{
		Type: "tcp",
		Host: "localhost",
		Port: "3000",
		ConnPool: config.TcpConnPoolConfigs{
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
