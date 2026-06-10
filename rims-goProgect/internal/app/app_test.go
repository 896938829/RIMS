// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestServeWithGracefulShutdownReturnsNilOnSignal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}
	signals := make(chan os.Signal, 1)
	errCh := make(chan error, 1)

	go func() {
		errCh <- serveWithGracefulShutdown(server, func() error {
			return server.Serve(listener)
		}, signals, time.Second)
	}()

	url := "http://" + listener.Addr().String()
	deadline := time.Now().Add(time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			closeErr := resp.Body.Close()
			if readErr != nil {
				t.Fatalf("read response: %v", readErr)
			}
			if closeErr != nil {
				t.Fatalf("close response: %v", closeErr)
			}
			if string(body) != "ok" {
				t.Fatalf("response body = %q, want ok", body)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start responding: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	signals <- os.Interrupt

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveWithGracefulShutdown returned %v, want nil", err)
		}
		if errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serveWithGracefulShutdown returned http.ErrServerClosed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serveWithGracefulShutdown did not return after signal")
	}

	if resp, err := http.Get(url); err == nil {
		_ = resp.Body.Close()
		t.Fatal("server still accepted requests after graceful shutdown")
	}
}
