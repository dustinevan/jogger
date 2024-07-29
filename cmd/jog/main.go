package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/dustinevan/jogger/cmd/jog/command"
	jogv1 "github.com/dustinevan/jogger/pkg/gen/jogger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("error: %s\n", err)
	}
}

func run() error {

	// ===============================================================================
	// Parse the command

	cmd, err := command.NewCommand(os.Args[1:])
	if err != nil {
		return err
	}
	if cmd.HelpWanted {
		fmt.Printf(command.Usage)
		return nil
	}

	// ===============================================================================
	// Check for required environment variables

	var caCertFile, userCertFile, userPrivateKeyFile string
	var missingVars []string
	if caCertFile = os.Getenv("JOGGER_CA_CERT_FILE"); caCertFile == "" {
		missingVars = append(missingVars, "JOGGER_CA_CERT_FILE")
	}
	if userCertFile = os.Getenv("JOGGER_USER_CERT_FILE"); userCertFile == "" {
		missingVars = append(missingVars, "JOGGER_USER_CERT_FILE")
	}
	if userPrivateKeyFile = os.Getenv("JOGGER_USER_KEY_FILE"); userPrivateKeyFile == "" {
		missingVars = append(missingVars, "JOGGER_USER_KEY_FILE")
	}
	if len(missingVars) > 0 {
		return fmt.Errorf("missing environment variables: \n\n\t%s\n\nfor more information see: jog --help", strings.Join(missingVars, "\n\t"))
	}

	var host string
	if cmd.Host != "" {
		host = cmd.Host
	} else {
		host = os.Getenv("JOGGER_HOST")
	}
	if host == "" {
		return errors.New("no host provided: use -D --host or set the JOGGER_HOST environment variable")
	}

	// ===============================================================================
	// Setup mTLS configuration

	userCert, err := tls.LoadX509KeyPair(userCertFile, userPrivateKeyFile)
	if err != nil {
		return fmt.Errorf("loading user key pair: %w", err)
	}

	certPool := x509.NewCertPool()
	caCertBytes, err := os.ReadFile(caCertFile)
	if err != nil {
		return fmt.Errorf("reading ca cert file: %w", err)
	}
	if ok := certPool.AppendCertsFromPEM(caCertBytes); !ok {
		return fmt.Errorf("loading cert pool: failed to append ca cert")
	}

	tlsConfig := &tls.Config{
		ServerName:   host,
		Certificates: []tls.Certificate{userCert},
		RootCAs:      certPool,
	}

	// ===============================================================================
	// Connect to the server

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	defer conn.Close()
	client := jogv1.NewJobServiceClient(conn)

	// ===============================================================================
	// Run the command

	ctx, cancel := context.WithCancel(context.Background())

	clientErr := make(chan error, 1)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		clientErr <- command.Run(ctx, client, cmd)
	}()

	// ===============================================================================
	// Listen For Shutdown

	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-terminate:
		cancel()
	case err = <-clientErr:
		// cancelling the context doesn't do anything in this case,
		// but we should guarantee that cancel is always called
		cancel()
	}

	wg.Wait()

	return err
}
