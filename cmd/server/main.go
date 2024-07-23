package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/dustinevan/jogger/cmd/server/api"
	"github.com/dustinevan/jogger/lib/job"
	joggerv1 "github.com/dustinevan/jogger/pkg/gen/jogger/v1"
	"github.com/dustinevan/jogger/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	// Set up the zap logger
	log, err := logger.New("JOGGER-SERVER")
	if err != nil {
		stdlog.Fatalf("setting up logger: %v", err)
	}
	// the zap logger is asynchronous, so we need to make sure it's flushed before the program exits
	defer log.Sync()
	if err := run(log); err != nil {
		log.Fatalf("running: %v", err)
	}
	log.Info("stopping service")
}

func run(log *zap.SugaredLogger) error {

	// ===============================================================================
	// Load Environment Variables
	// github.com/ardanlabs/conf/v3 automatically loads these environment variables
	// it also automatically sets up command flags for each of these variables
	// use --help to see the available flags

	log.Infow("starting service", "configuration", "initializing")
	cfg := struct {
		Authen struct {
			CACertFile     string `conf:"env:JOGGER_CA_CERT_FILE,default:certs/ca_tls.crt"`
			ServerCertFile string `conf:"env:JOGGER_SERVER_CERT_FILE,default:certs/server1_tls.crt"`
			ServerKeyFile  string `conf:"env:JOGGER_SERVER_KEY_FILE,default:certs/server1_tls.key"`
		}
		Server struct {
			Port int `conf:"env:JOGGER_SERVER_PORT,default:50051"`
		}
	}{}

	log.Infow("starting service", "configuration", "parsing")

	help, err := conf.Parse("", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(help)
			return nil
		}
		return fmt.Errorf("parsing config: %w", err)
	}
	cfgString, err := conf.String(&cfg)
	if err != nil {
		return fmt.Errorf("config to string: %w", err)
	}

	log.Infow("starting service", "configuration\n", cfgString)

	// ===============================================================================
	// mTLS Configuration

	log.Infow("starting service", "configuration", "loading server credentials")

	serverCert, err := tls.LoadX509KeyPair(cfg.Authen.ServerCertFile, cfg.Authen.ServerKeyFile)
	if err != nil {
		return fmt.Errorf("loading server key pair: %w", err)
	}

	certPool := x509.NewCertPool()
	caCertBytes, err := os.ReadFile(cfg.Authen.CACertFile)
	if err != nil {
		return fmt.Errorf("reading ca cert file: %w", err)
	}
	if !certPool.AppendCertsFromPEM(caCertBytes) {
		return fmt.Errorf("loading cert pool: failed to append ca cert")
	}

	tlsConfig := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    certPool,
	}

	log.Infow("starting service", "configuration", "done")

	// ===============================================================================
	// Graceful Shutdown

	shutdownCtx, shutdown := context.WithCancel(context.Background())

	// ===============================================================================
	// Start Server

	log.Infow("starting service", "initializing", "grpc server")

	jobManager := job.NewManager(shutdownCtx)

	joggerServer := api.NewServer(jobManager)

	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	joggerv1.RegisterJobServiceServer(server, joggerServer)

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", cfg.Server.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Infow("starting service", "listening", fmt.Sprintf("localhost:%d", cfg.Server.Port))
		serverErr <- server.Serve(lis)
	}()

	// ===============================================================================
	// Wait for Shutdown

	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-terminate:
		log.Infow("stopping service", "signal", sig)
	case err = <-serverErr:
		log.Infow("stopping service", "error", err)
	}
	// Canceling the shutdownCtx cancels the context for each job. Each running job will
	// receive a SIGTERM and be given a chance to gracefully shutdown. After the WaitDelay
	// period, the jobs will be sent a SIGKILL.
	shutdown()

	// this shutdown is set to be 5 seconds longer than the wait delay for the jobs
	// this should be configurable in the future
	shutdownTimeout := 15 * time.Second

	// shutdown the server in a goroutine so we can time out
	done := make(chan struct{})
	defer close(done)
	go func() {
		server.GracefulStop()
		done <- struct{}{}
	}()

	// wait for one of the shutdown conditions to be met
	select {
	case <-terminate:
		// the user has sent another terminate signal -- force shutdown
		server.Stop()
		log.Infow("stopping service", "status", "forced shutdown")
	case <-done:
		log.Infow("stopping service", "status", "graceful shutdown complete")
	case <-time.After(shutdownTimeout):
		server.Stop()
		log.Infow("stopping service", "status", "forced shutdown")
	}
	return nil
}
