package tcpserver

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/squaredcow/yats/pkg/config"
	"github.com/squaredcow/yats/pkg/handler"
	"go.uber.org/zap"
)

// TcpServer yet another tcp server, there you go.
type TcpServer struct {
	log          *zap.Logger
	cfg          config.TcpServerConfigs
	keepAlive    sync.WaitGroup
	shutdownHook chan struct{}
	signalHook   chan os.Signal
	connPool     chan chan net.Conn
	connHandler  handler.ConnHandler
}

func NewTcpServer(logger *zap.Logger, config config.TcpServerConfigs, h handler.ConnHandler) *TcpServer {
	return &TcpServer{
		log:          logger,
		cfg:          config,
		keepAlive:    sync.WaitGroup{},
		shutdownHook: make(chan struct{}),
		signalHook:   make(chan os.Signal, 1),
		connPool:     make(chan chan net.Conn, config.ConnPool.MaxSize),
		connHandler:  h,
	}
}

func (s *TcpServer) Start(ctx context.Context) {
	s.log.Info("on_start: init server ...")

	ln, err := net.Listen(s.cfg.Type, ":"+s.cfg.Port)
	if err != nil {
		s.log.Error("on_start: unable to init server",
			zap.String("error", err.Error()))
	}

	s.log.Info("on_start: listening to",
		zap.String("address", ln.Addr().String()))

	// WaitGroup: tcp_server
	s.keepAlive.Add(1)

	// goroutine: tcp_connection_worker
	go s.startConnectionWorker(ctx)

	// goroutine: tcp_network_listener
	go s.startNetworkListener(ln)

	// goroutine: signal
	signal.Notify(s.signalHook, syscall.SIGINT, syscall.SIGTERM)
	go s.onShutdown()

	s.log.Info("on_start: server up and running")
	s.keepAlive.Wait()
}

func (s *TcpServer) Close() {
	s.log.Info("on_close: shutting down server ...")
	close(s.shutdownHook)
	close(s.connPool)
	close(s.signalHook)
	s.keepAlive.Done()
	s.log.Info("on_close: bye, bye")
}

func (s *TcpServer) startConnectionWorker(ctx context.Context) {
	s.log.Debug("tcp_connection_worker: start")

	// WaitGroup: register tcp_connection_worker
	s.keepAlive.Add(1)

	defer func() {
		s.log.Debug("tcp_connection_worker: close")
		if r := recover(); r != nil {
			s.log.Error("tcp_connection_worker: recover from panic",
				zap.Any("any", r))
			s.keepAlive.Done()
			return
		}
		s.log.Debug("tcp_connection_worker: done")
		s.keepAlive.Done()
	}()

	for connChan := range s.connPool {
		select {
		case <-s.shutdownHook:
			// On shutdown signal, force exit
			s.log.Debug("tcp_connection_worker: shutdown")
			return
		default:
			// goroutine: on connection
			conn := <-connChan
			s.log.Debug("tcp_connection_worker: handle connection",
				zap.Any("localAddress", conn.LocalAddr()), zap.Any("remoteAddress", conn.RemoteAddr()))
			go s.onConnection(ctx, conn)
		}
	}
}

func (s *TcpServer) onConnection(ctx context.Context, conn net.Conn) {
	s.log.Debug("tcp_connection: start")

	// WaitGroup: tcp_connection
	s.keepAlive.Add(1)

	// Settings
	noop := time.Duration(s.cfg.ConnPool.NoOpCycle) * time.Millisecond

	defer func() {
		if r := recover(); r != nil {
			s.log.Error("tcp_connection: recover from panic",
				zap.Any("any", r))
			s.keepAlive.Done()
			return
		}
		s.log.Debug("tcp_connection: done")
		s.keepAlive.Done()
	}()

	defer func(conn net.Conn) {
		s.log.Debug("tcp_connection: close")
		err := conn.Close()
		if err != nil {
			s.log.Error("tcp_connection: unable to close connection",
				zap.String("error", err.Error()))
		}
	}(conn)

	for {
		select {
		case <-s.shutdownHook:
			// On shutdown signal, force exit
			s.log.Debug("tcp_connection: shutdown")
			return
		default:
			traceID := uuid.NewString()

			s.log.With(zap.String("traceId", traceID)).
				Debug("tcp_connection: delegate connection to handler")

			connCtx := context.WithValue(ctx, ctxTraceID, traceID)

			s.connHandler.Handle(connCtx, conn)

			// no-op cycle: this will alleviate the CPU usage.
			time.Sleep(noop)
		}
	}
}

func (s *TcpServer) startNetworkListener(ln net.Listener) {
	s.log.Debug("tcp_network_listener: start")

	// WaitGroup: tcp_network_listener
	s.keepAlive.Add(1)

	// Settings
	ttl := time.Duration(s.cfg.ConnPool.WaitForConnTimeout) * time.Millisecond

	defer func() {
		s.log.Debug("tcp_network_listener: close")
		if r := recover(); r != nil {
			s.log.Error("tcp_network_listener: recover from panic",
				zap.Any("any", r))
			s.keepAlive.Done()
			return
		}
		s.log.Debug("tcp_network_listener: done")
		s.keepAlive.Done()
	}()

	for {
		select {
		case <-s.shutdownHook:
			// On shutdown signal, force exit
			s.log.Debug("tcp_network_listener: shutdown")
			return
		default:
			s.log.Debug("tcp_network_listener: listening for incoming connection")

			// Set timeout for waiting on the next connection to prevent infinite blocking
			tcp, _ := ln.(*net.TCPListener)
			timeout := time.Now().Add(ttl)
			err := tcp.SetDeadline(timeout)
			if err != nil {
				s.log.Error("tcp_network_listener: unable to set timeout",
					zap.String("error", err.Error()))
				return
			}

			// Get next connection or skip on error
			conn, err := ln.Accept()
			if err != nil {
				var netOpErr *net.OpError
				ok := errors.As(err, &netOpErr)
				if ok && netOpErr.Timeout() {
					s.log.Debug("tcp_network_listener: timeout waiting for connection",
						zap.String("error", err.Error()))
					continue
				}

				s.log.Error("tcp_network_listener: incoming connection rejected",
					zap.String("error", err.Error()))
				continue
			}

			// Connection isolation
			// Send the next available connection -> jobChannel -> workerPool
			jobChannel := make(chan net.Conn)
			s.connPool <- jobChannel
			jobChannel <- conn

			s.log.Debug("tcp_network_listener: accepted incoming connection")
		}
	}
}

func (s *TcpServer) onShutdown() {
	s.log.Debug("on_shutdown: listen for shutdown signals")

	// WaitGroup: on_shutdown
	s.keepAlive.Add(1)

	// Settings
	noop := time.Duration(s.cfg.ConnPool.NoOpCycle) * time.Millisecond

	defer func() {
		s.log.Debug("on_shutdown: received shutdown signal")
		if r := recover(); r != nil {
			s.log.Error("on_shutdown: recover from panic:",
				zap.Any("any", r))
			s.keepAlive.Done()
			return
		}
		s.log.Debug("on_shutdown: done")
		s.keepAlive.Done()
	}()

	for {
		select {
		case <-s.signalHook:
			s.Close()
			return
		default:
			// no-op cycle: this will alleviate the CPU usage.
			time.Sleep(noop)
		}
	}
}
