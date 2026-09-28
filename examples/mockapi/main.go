package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const (
	expectedAPIKeyEnv         = "CHAB_MOCK_EXPECTED_API_KEY_SHA256"
	expectedIdempotencyKeyEnv = "CHAB_MOCK_EXPECTED_IDEMPOTENCY_KEY_SHA256"
)

// fingerprintExpectations enables value verification without giving the mock
// process the credential or idempotency key it is expected to accept.
type fingerprintExpectations struct {
	enabled        bool
	apiKey         [32]byte
	idempotencyKey [32]byte
}

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "address to listen on")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "Deterministic local mock of the Chab-SaaS /v1 contract for executable examples.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "This is not a production server. In normal manual use it accepts any non-empty bearer")
		fmt.Fprintln(out, "token and idempotency key. It serves fixed")
		fmt.Fprintln(out, "credits, /me, rate-limit, and error fixtures, and exposes admin endpoints")
		fmt.Fprintln(out, "GET /readyz, POST /__mock/reset, and GET /__mock/requests.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "On startup it prints exactly one stdout line: LISTEN_ADDR=<host:port>.")
		fmt.Fprintln(out, "Operational logs go to stderr and redact Authorization and idempotency values.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Flags:")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "mockapi: unexpected positional arguments")
		os.Exit(1)
	}
	logger := log.New(os.Stderr, "mockapi: ", 0)
	if err := serveConfigured(*listen, os.Stdout, logger, os.LookupEnv); err != nil && !isServerClosed(err) {
		fmt.Fprintf(os.Stderr, "mockapi: %v\n", err)
		os.Exit(1)
	}
}

// serveConfigured loads the optional verification pair before opening a
// listener. Configuration errors stay generic so supplied values cannot enter
// startup diagnostics.
func serveConfigured(addr string, stdout io.Writer, logger *log.Logger, lookupEnv func(string) (string, bool)) error {
	expectations, err := loadFingerprintExpectations(lookupEnv)
	if err != nil {
		return fmt.Errorf("invalid fingerprint configuration")
	}
	return serveUntilSignal(addr, stdout, logger, expectations)
}

// loadFingerprintExpectations keeps manual mode when both values are absent
// and enables verification only for a complete, valid pair.
func loadFingerprintExpectations(lookupEnv func(string) (string, bool)) (fingerprintExpectations, error) {
	apiValue, apiPresent := lookupEnv(expectedAPIKeyEnv)
	idempotencyValue, idempotencyPresent := lookupEnv(expectedIdempotencyKeyEnv)
	if !apiPresent && !idempotencyPresent {
		return fingerprintExpectations{}, nil
	}
	if !apiPresent || !idempotencyPresent {
		return fingerprintExpectations{}, fmt.Errorf("fingerprint configuration must be a complete pair")
	}
	apiFingerprint, err := decodeFingerprint(apiValue)
	if err != nil {
		return fingerprintExpectations{}, err
	}
	idempotencyFingerprint, err := decodeFingerprint(idempotencyValue)
	if err != nil {
		return fingerprintExpectations{}, err
	}
	return fingerprintExpectations{
		enabled:        true,
		apiKey:         apiFingerprint,
		idempotencyKey: idempotencyFingerprint,
	}, nil
}

// decodeFingerprint accepts either hexadecimal case while requiring one full
// SHA-256 value.
func decodeFingerprint(value string) ([32]byte, error) {
	var fingerprint [32]byte
	if len(value) != hex.EncodedLen(len(fingerprint)) {
		return fingerprint, fmt.Errorf("invalid fingerprint")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(fingerprint) {
		return fingerprint, fmt.Errorf("invalid fingerprint")
	}
	copy(fingerprint[:], decoded)
	return fingerprint, nil
}

// serveUntilSignal runs the mock until it receives SIGINT/SIGTERM or the HTTP
// server exits. Tests call it directly to avoid shelling out for lifecycle cases.
func serveUntilSignal(addr string, stdout io.Writer, logger *log.Logger, expectations fingerprintExpectations) error {
	ln, srv, err := listenAndAnnounce(addr, stdout, logger, expectations)
	if err != nil {
		return err
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		if logger != nil {
			logger.Printf("received %s, shutting down", sig)
		}
		_ = srv.Shutdown(context.Background())
		err := <-errCh
		if isServerClosed(err) {
			return nil
		}
		return err
	case err := <-errCh:
		if isServerClosed(err) {
			return nil
		}
		return err
	}
}

// listenAndAnnounce binds the listener before printing LISTEN_ADDR. The checker
// treats stdout as a one-line startup protocol, so operational logs stay on stderr.
func listenAndAnnounce(addr string, stdout io.Writer, logger *log.Logger, configured ...fingerprintExpectations) (net.Listener, *http.Server, error) {
	var expectations fingerprintExpectations
	if len(configured) > 0 {
		expectations = configured[0]
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	if _, err := fmt.Fprintf(stdout, "LISTEN_ADDR=%s\n", ln.Addr().String()); err != nil {
		_ = ln.Close()
		return nil, nil, err
	}
	if flusher, ok := stdout.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			_ = ln.Close()
			return nil, nil, err
		}
	}

	return ln, &http.Server{Handler: newServer(logger, expectations)}, nil
}

// isServerClosed normalizes the standard shutdown result and the listener
// close wording returned by some supported platforms.
func isServerClosed(err error) bool {
	if err == nil {
		return false
	}
	return err == http.ErrServerClosed || strings.Contains(err.Error(), "use of closed network connection")
}
