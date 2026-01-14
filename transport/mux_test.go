package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMuxString(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientTr, serverTr := NewBufferPair(ctx, 16)

	// Server runs the Mux with string ID
	errCh := make(chan error, 1)
	go func() {
		errCh <- Mux(serverTr, MuxConfig[string]{
			Handler: func(id string, t Transport) error {
				for {
					var msg string
					if err := t.ReadJSON(&msg); err != nil {
						return err
					}
					if msg != "ping" {
						return errors.New("expected ping")
					}
					if err := t.WriteJSON("pong"); err != nil {
						return err
					}
				}
			},
			Untagged: func(msg json.RawMessage) error {
				var s string
				if err := json.Unmarshal(msg, &s); err != nil {
					return err
				}
				if s != "raw-ping" {
					return errors.New("expected raw-ping")
				}
				// Reply manually via serverTr
				return serverTr.WriteJSON(struct {
					P any `json:"p"`
				}{
					P: "raw-pong",
				})
			},
		})
	}()

	// 1. Client manually sends "ping" to "foo"
	err := clientTr.WriteJSON(struct {
		ID string `json:"id"`
		P  any    `json:"p"`
	}{
		ID: "foo",
		P:  "ping",
	})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// 2. Client expects "pong" from "foo"
	var resp struct {
		ID string `json:"id"`
		P  string `json:"p"`
	}
	if err := clientTr.ReadJSON(&resp); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if resp.ID != "foo" || resp.P != "pong" {
		t.Errorf("got %+v, want foo/pong", resp)
	}

	// 3. Client manually sends "raw-ping" (untagged)
	// Protocol: {"p": "raw-ping"} (id explicitly empty to override sticky lastID)
	err = clientTr.WriteJSON(struct {
		ID string `json:"id"`
		P  any    `json:"p"`
	}{
		ID: "",
		P:  "raw-ping",
	})
	if err != nil {
		t.Fatalf("WriteJSON untagged failed: %v", err)
	}

	// 4. Client expects "raw-pong" (untagged)
	resp = struct {
		ID string `json:"id"`
		P  string `json:"p"`
	}{} // reset
	if err := clientTr.ReadJSON(&resp); err != nil {
		t.Fatalf("ReadJSON untagged failed: %v", err)
	}
	if resp.ID != "" || resp.P != "raw-pong" {
		t.Errorf("got %+v, want \"\"/raw-pong", resp)
	}

	// 5. Close client (simulating disconnect)
	cancel()

	// 6. Wait for Run to exit
	err = <-errCh
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Logf("Run exit error (expected): %v", err)
	}
}

func TestMuxStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientTr, serverTr := NewBufferPair(ctx, 16)

	// Server runs the Mux
	go func() {
		Mux(serverTr, MuxConfig[string]{
			Handler: func(id string, t Transport) error {
				// Block until context is done (cancelled by incoming stop)
				<-t.Context().Done()
				return t.Context().Err() // Return the cancellation error
			},
		})
	}()

	// 1. Client initiates "foo" by sending a stop (simulating a close)
	// Or rather, client sends a message then a stop.
	// But let's just send a stop. The server won't know about it unless it's already active?
	// The server creates handler on first message. Stop doesn't create.
	// So send a message first.
	err := clientTr.WriteJSON(struct {
		ID string `json:"id"`
		P  any    `json:"p"`
	}{
		ID: "foo",
		P:  "hello",
	})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// Give server a moment to spawn handler
	time.Sleep(10 * time.Millisecond)

	// 2. Client sends stop for "foo"
	reason := "client done"
	err = clientTr.WriteJSON(struct {
		ID   string  `json:"id"`
		Stop *string `json:"stop"`
	}{
		ID:   "foo",
		Stop: &reason,
	})
	if err != nil {
		t.Fatalf("WriteJSON stop failed: %v", err)
	}

	// TODO: We don't expect a stop back - how do we validate the server stopped us?
}

func TestMuxIDReuse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientTr, serverTr := NewBufferPair(ctx, 16)

	handlerCount := 0
	// Server runs the Mux
	go func() {
		Mux(serverTr, MuxConfig[string]{
			Handler: func(id string, t Transport) error {
				handlerCount++
				<-t.Context().Done()
				return nil
			},
		})
	}()

	// 1. Client initiates "foo"
	err := clientTr.WriteJSON(struct {
		ID string `json:"id"`
		P  any    `json:"p"`
	}{
		ID: "foo",
		P:  "hello",
	})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// Give server a moment to spawn handler
	time.Sleep(10 * time.Millisecond)

	// 2. Client sends stop for "foo"
	reason := "client done"
	err = clientTr.WriteJSON(struct {
		ID   string  `json:"id"`
		Stop *string `json:"stop"`
	}{
		ID:   "foo",
		Stop: &reason,
	})
	if err != nil {
		t.Fatalf("WriteJSON stop failed: %v", err)
	}

	// 3. Client immediately re-initiates "foo"
	err = clientTr.WriteJSON(struct {
		ID string `json:"id"`
		P  any    `json:"p"`
	}{
		ID: "foo",
		P:  "hello again",
	})
	if err != nil {
		t.Fatalf("WriteJSON re-initiate failed: %v", err)
	}

	// Give server a moment
	time.Sleep(20 * time.Millisecond)

	if handlerCount != 2 {
		t.Errorf("expected 2 handler calls, got %d", handlerCount)
	}
}

func TestMuxServerStopAndReuse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientTr, serverTr := NewBufferPair(ctx, 16)

	handlerCount := 0
	// Server runs the Mux
	go func() {
		Mux(serverTr, MuxConfig[string]{
			Handler: func(id string, t Transport) error {
				handlerCount++
				// Read one message then return (server closes)
				var msg string
				if err := t.ReadJSON(&msg); err != nil {
					return err
				}
				if msg != "ping" {
					return fmt.Errorf("expected ping, got %s", msg)
				}
				// Return nil -> sends Stop to client
				return nil
			},
		})
	}()

	// 1. Client initiates "foo" with "ping"
	err := clientTr.WriteJSON(struct {
		ID string `json:"id"`
		P  any    `json:"p"`
	}{
		ID: "foo",
		P:  "ping",
	})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// 2. Client waits for Stop from server
	// We expect "stop" in the response
	var resp struct {
		ID   string  `json:"id"`
		Stop *string `json:"stop"`
	}
	// We might get other messages if we had any, but here we expect just stop?
	// The server doesn't write anything else.
	if err := clientTr.ReadJSON(&resp); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if resp.ID != "foo" || resp.Stop == nil {
		t.Fatalf("expected stop for foo, got %+v", resp)
	}

	// 3. Client initiates "foo" with "ping" AGAIN
	err = clientTr.WriteJSON(struct {
		ID string `json:"id"`
		P  any    `json:"p"`
	}{
		ID: "foo",
		P:  "ping",
	})
	if err != nil {
		t.Fatalf("WriteJSON 2 failed: %v", err)
	}

	// 4. Client waits for Stop from server (the second handler instance)
	// If the server didn't clean up, the second message is ignored, and we timeout here.
	// We need a timeout or non-blocking check.
	// ReadJSON blocks.

	// To avoid test hanging forever, we can use a done channel or similar, but ReadJSON takes a blocking approach.
	// I can close the context after a delay if I want to fail.

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	resp = struct {
		ID   string  `json:"id"`
		Stop *string `json:"stop"`
	}{} // reset
	if err := clientTr.ReadJSON(&resp); err != nil {
		if errors.Is(err, context.Canceled) {
			t.Errorf("timed out waiting for second response (server likely ignored the message), err was=%+v", err)
		}
		t.Errorf("ReadJSON 2 failed: %v", err)
	}
	if resp.ID != "" || resp.Stop == nil {
		// ID should be blank because it's a re-use
		t.Errorf("expected stop for foo 2, got %+v", resp)
	}

	if handlerCount != 2 {
		t.Errorf("expected 2 handler calls, got %d", handlerCount)
	}
}
