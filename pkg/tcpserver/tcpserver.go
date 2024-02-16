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

// TcpServer a simple tcp server, there you go.
type TcpServer struct {
	log          *zap.SugaredLogger
	cfg          config.TcpServerConfigs
	keepAlive    sync.WaitGroup
	shutdownHook chan struct{}
	signalHook   chan os.Signal
	connPool     chan chan net.Conn
	connHandler  handler.ConnHandler
}

func NewTcpServer(logger *zap.SugaredLogger, config config.TcpServerConfigs, h handler.ConnHandler) *TcpServer {
	return &TcpServer{
		log:          logger,
		cfg:          config,
		keepAlive:    sync.WaitGroup{},
		shutdownHook: make(chan struct{}),
		signalHook:   make(chan os.Signal, 1),
		connPool:     make(chan chan net.Conn, config.Pool.MaxSize),
		connHandler:  h,
	}
}

func (s *TcpServer) Start(ctx context.Context) {
	s.log.Info("tcp_server::on_start:: bootstrap routines")

	ln, err := net.Listen(s.cfg.Type, ":"+s.cfg.Port)
	if err != nil {
		s.log.Error("tcp_server::on_start:: unable to listen local network address: %v", err.Error())
	}

	s.log.Info("tcp_server::on_start:: listening on: %s", ln.Addr().String())

	// WaitGroup: tcp_server
	s.keepAlive.Add(1)

	// goroutine: tcp_connection_worker
	go s.startConnectionWorker(ctx)

	// goroutine: tcp_network_listener
	go s.startNetworkListener(ln)

	// goroutine: signal
	signal.Notify(s.signalHook, syscall.SIGINT, syscall.SIGTERM)
	go s.onShutdown()

	s.log.Infof("tcp_server::on_start:: up and running")
	s.keepAlive.Wait()
}

func (s *TcpServer) Close() {
	s.log.Infof("tcp_server: on_close: shutting down (gracefully)")
	close(s.shutdownHook)
	close(s.connPool)
	close(s.signalHook)
	s.keepAlive.Done()
}

func (s *TcpServer) startConnectionWorker(ctx context.Context) {
	s.log.Debugf("tcp_connection_worker: start")

	// WaitGroup: register tcp_connection_worker
	s.keepAlive.Add(1)

	defer func() {
		s.log.Debugf("tcp_connection_worker: close")
		if r := recover(); r != nil {
			s.log.Errorf("tcp_connection_worker: recover from panic: %v", r)
			s.keepAlive.Done()
			return
		}
		s.log.Debugf("tcp_connection_worker: done")
		s.keepAlive.Done()
	}()

	for connChan := range s.connPool {
		select {
		case <-s.shutdownHook:
			// On shutdown signal, force exit
			s.log.Debugf("tcp_connection_worker: shutdown signal received")
			return
		default:
			// goroutine: on connection
			s.log.Debugf("tcp_connection_worker: handle available connection")
			go s.onConnection(ctx, <-connChan)
		}
	}
}

func (s *TcpServer) onConnection(ctx context.Context, conn net.Conn) {
	s.log.Debugf("tcp_connection: start")

	// WaitGroup: tcp_connection
	s.keepAlive.Add(1)

	// Settings
	noop := time.Duration(s.cfg.Pool.NoOpCycle) * time.Millisecond

	defer func() {
		if r := recover(); r != nil {
			s.log.Errorf("tcp_connection: recover from panic: %v", r)
			s.keepAlive.Done()
			return
		}
		s.log.Debugf("tcp_connection: done")
		s.keepAlive.Done()
	}()

	defer func(conn net.Conn) {
		s.log.Debugf("tcp_connection: close")
		err := conn.Close()
		if err != nil {
			s.log.Errorf("tcp_connection: unable to close connection: %v", err.Error())
		}
	}(conn)

	for {
		select {
		case <-s.shutdownHook:
			// On shutdown signal, force exit
			s.log.Debugf("tcp_connection: shutdown signal received")
			return
		default:
			traceID := uuid.NewString()
			s.log.With("traceId", traceID).Debugf("tcp_connection: delegate to connection handler")
			connCtx := context.WithValue(ctx, "traceId", traceID)

			s.connHandler.Handle(connCtx, conn)

			// no-op cycle: this will alleviate the CPU usage.
			time.Sleep(noop)
		}
	}
}

func (s *TcpServer) startNetworkListener(ln net.Listener) {
	s.log.Debugf("tcp_network_listener: start")

	// WaitGroup: tcp_network_listener
	s.keepAlive.Add(1)

	// Settings
	ttl := time.Duration(s.cfg.Pool.WaitForConnTimeout) * time.Millisecond

	defer func() {
		s.log.Debugf("tcp_network_listener: close")
		if r := recover(); r != nil {
			s.log.Errorf("tcp_network_listener: recover from panic: %v", r)
			s.keepAlive.Done()
			return
		}
		s.log.Debugf("tcp_network_listener: done")
		s.keepAlive.Done()
	}()

	for {
		select {
		case <-s.shutdownHook:
			// On shutdown signal, force exit
			s.log.Debugf("tcp_network_listener: shutdown signal received")
			return
		default:
			s.log.Debugf("tcp_network_listener: listening for incoming connection")

			// Set timeout for waiting on the next connection to prevent infinite blocking
			tcp, _ := ln.(*net.TCPListener)
			timeout := time.Now().Add(ttl)
			err := tcp.SetDeadline(timeout)
			if err != nil {
				s.log.Errorf("tcp_network_listener: unable to set timeout: %v", err.Error())
				return
			}

			// Get next connection or skip on error
			conn, err := ln.Accept()
			if err != nil {
				var netOpErr *net.OpError
				ok := errors.As(err, &netOpErr)
				if ok && netOpErr.Timeout() {
					s.log.Debugf("tcp_network_listener: timeout waiting: %v", err.Error())
					continue
				}

				s.log.Errorf("tcp_network_listener: incoming connection rejected: %v", err.Error())
				continue
			}

			// Connection isolation
			// Send the next available connection -> jobChannel -> workerPool
			jobChannel := make(chan net.Conn)
			s.connPool <- jobChannel
			jobChannel <- conn

			s.log.Debugf("tcp_network_listener: accepted incoming connection")
		}
	}
}

func (s *TcpServer) onShutdown() {
	s.log.Debugf("on_shutdown: listen for shutdown signals")

	// WaitGroup: on_shutdown
	s.keepAlive.Add(1)

	// Settings
	noop := time.Duration(s.cfg.Pool.NoOpCycle) * time.Millisecond

	defer func() {
		s.log.Debugf("on_shutdown: received shutdown signal")
		if r := recover(); r != nil {
			s.log.Errorf("on_shutdown: recover from panic: %v", r)
			s.keepAlive.Done()
			return
		}
		s.log.Debugf("on_shutdown: done")
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
