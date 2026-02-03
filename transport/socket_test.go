package transport

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func connForTest(t *testing.T, opts SocketOpts, handle Handler) (c *websocket.Conn) {
	// Setup server
	handler := NewWebSocketHandler(opts, handle)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Connect client
	c, _, err := websocket.Dial(t.Context(), srv.URL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { c.Close(websocket.StatusNormalClosure, "") })
	return c
}

func TestSocketHandshakeAndEcho(t *testing.T) {
	c := connForTest(t, SocketOpts{}, func(tr Transport) (err error) {
		var msg string
		if err := tr.ReadJSON(&msg); err != nil {
			t.Errorf("server read error: %v", err)
			return err
		}
		if err := tr.WriteJSON("echo:" + msg); err != nil {
			t.Errorf("server write error: %v", err)
			return err
		}
		return nil
	})

	// 1. Perform Handshake
	// Send Hello
	hello := map[string]string{
		"type":    "hello",
		"version": "1",
	}
	if err := wsjson.Write(t.Context(), c, hello); err != nil {
		t.Fatalf("failed to write hello: %v", err)
	}

	// Expect Response
	var resp HandshakeResponse
	if err := wsjson.Read(t.Context(), c, &resp); err != nil {
		t.Fatalf("failed to read handshake response: %v", err)
	}

	if !resp.Ok {
		t.Errorf("handshake not ok")
	}
	if resp.MaxPacketSize != DefaultMaxPacketSize {
		t.Errorf("got max packet size %d, want %d", resp.MaxPacketSize, DefaultMaxPacketSize)
	}

	// 2. Test Data Transfer
	// Send "test"
	if err := wsjson.Write(t.Context(), c, "test"); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	// Expect "echo:test"
	var out string
	if err := wsjson.Read(t.Context(), c, &out); err != nil {
		t.Fatalf("failed to read echo: %v", err)
	}

	if out != "echo:test" {
		t.Errorf("got %q, want %q", out, "echo:test")
	}
}

func TestSocketHandshakeFail(t *testing.T) {
	// Setup server that does nothing (handshake is handled by NewWebSocketHandler)
	c := connForTest(t, SocketOpts{}, func(tr Transport) (err error) { return nil })

	// Case 1: Wrong Version
	// Send Wrong Hello
	badHello := map[string]string{
		"type":    "hello",
		"version": "2", // Unsupported
	}
	if err := wsjson.Write(t.Context(), c, badHello); err != nil {
		t.Fatalf("failed to write hello: %v", err)
	}

	var dummy map[string]any
	err := wsjson.Read(t.Context(), c, &dummy)
	if err == nil {
		t.Fatal("expected error reading after bad handshake, got nil")
	}

	// verify result
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("got unexpected close status=%d err=%+v", websocket.CloseStatus(err), err)
	}
}

func TestSocketRateLimit(t *testing.T) {
	// Setup server with very strict rate limits
	c := connForTest(t, SocketOpts{
		RateLimit: 1, // 1 per second
		RateBurst: 2, // burst of 2
	}, func(tr Transport) error {
		// Just consume
		var msg string
		for {
			if err := tr.ReadJSON(&msg); err != nil {
				return err
			}
		}
	})

	// Handshake
	hello := map[string]string{"type": "hello", "version": "1"}
	if err := wsjson.Write(t.Context(), c, hello); err != nil {
		t.Fatalf("failed handshake write: %v", err)
	}
	var resp HandshakeResponse
	if err := wsjson.Read(t.Context(), c, &resp); err != nil {
		t.Fatalf("failed handshake read: %v", err)
	}

	// We can send 2 messages (burst) + 1 (allowance over time) safely?
	// Actually, limiter starts full.
	// Burst is 2.
	// Send 1 (OK, tokens=1)
	// Send 1 (OK, tokens=0)
	// Send 1 (Fail, tokens=0, refill is slow)
	for i := range 3 {
		key := fmt.Sprintf("msg%d", i)
		if err := wsjson.Write(t.Context(), c, key); err != nil {
			t.Fatalf("%s failed: %v", key, err)
		}
	}

	// Now try to read something or just wait for close
	// The server should close the connection.
	// We might be able to write more before the TCP connection actually tears down,
	// but eventually we should get a close error when reading or writing.

	// Wait a bit to ensure server processed msg
	time.Sleep(100 * time.Millisecond)

	// Try to write again, or read. Reading should return EOF/Close.
	// Writing might succeed if the kernel buffer isn't full and RST hasn't arrived.
	// Best check is to read.
	var dummy string
	err := wsjson.Read(t.Context(), c, &dummy)
	if err == nil {
		t.Fatalf("expected error (disconnect) after rate limit exceeded, got nil")
	}

	// Check if it's a close error
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Errorf("expected close status %v, got %v", websocket.StatusPolicyViolation, websocket.CloseStatus(err))
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("expected error to contain 'rate limit exceeded', got: %v", err)
	}
}

func TestSocketTransportError(t *testing.T) {
	// Setup server that returns a TransportError
	c := connForTest(t, SocketOpts{}, func(tr Transport) (err error) {
		return TransportError{
			Code:   1234,
			Reason: "test / reason",
		}
	})

	// Handshake
	hello := map[string]string{"type": "hello", "version": "1"}
	if err := wsjson.Write(t.Context(), c, hello); err != nil {
		t.Fatalf("failed handshake write: %v", err)
	}
	var resp HandshakeResponse
	if err := wsjson.Read(t.Context(), c, &resp); err != nil {
		t.Fatalf("failed handshake read: %v", err)
	}

	// Try to read something, expect close
	var dummy string
	err := wsjson.Read(t.Context(), c, &dummy)
	if err == nil {
		t.Fatal("expected error (disconnect) after handler returned TransportError, got nil")
	}

	// Check status code
	if websocket.CloseStatus(err) != websocket.StatusCode(3000) {
		t.Errorf("expected close status 3000, got %v (err=%+v)", websocket.CloseStatus(err), err)
	}

	// Check reason (TransportError formatted as code/message)
	var closeErr websocket.CloseError
	if errors.As(err, &closeErr) {
		te := DecodeTransportError(closeErr.Reason)
		if te.Code != 1234 {
			t.Errorf("got code %d, want 1234", te.Code)
		}
		if te.Reason != "test / reason" {
			t.Errorf("got message %q, want 'test / reason'", te.Reason)
		}
	} else {
		t.Errorf("error is not a CloseError: %v", err)
	}
}

func TestJSONEncodingError(t *testing.T) {
	// Setup server that tries to write malformed JSON
	c := connForTest(t, SocketOpts{}, func(tr Transport) error {
		// Wait for client to say something? No need.
		// Just try to write something bad.
		// channel is not serializable.
		badData := make(chan int)
		if err := tr.WriteJSON(badData); err == nil {
			t.Errorf("WriteJSON should have returned error")
		}

		// The connection should now be dying.
		// Wait on context.
		<-tr.Context().Done()
		return nil
	})

	// Handshake
	hello := map[string]string{"type": "hello", "version": "1"}
	if err := wsjson.Write(t.Context(), c, hello); err != nil {
		t.Fatalf("failed handshake write: %v", err)
	}
	var resp HandshakeResponse
	if err := wsjson.Read(t.Context(), c, &resp); err != nil {
		t.Fatalf("failed handshake read: %v", err)
	}

	// Expect the connection to close from server side
	var dummy string
	err := wsjson.Read(t.Context(), c, &dummy)
	if err == nil {
		t.Error("expected error (disconnect) due to server JSON failure, got nil")
	}

	if websocket.CloseStatus(err) != websocket.StatusInternalError {
		t.Errorf("expected close status %v, got %v (%v)", websocket.StatusInternalError, websocket.CloseStatus(err), err)
	}
}

func TestControlPacket(t *testing.T) {
	// Server handler
	handler := func(tr Transport) error {
		// 1. Read ControlPacket
		var pkt ControlPacket[string]
		if err := tr.ReadJSON(&pkt); err != nil {
			return fmt.Errorf("read cp error: %w", err)
		}
		if pkt.C == nil || *pkt.C != 123 {
			return fmt.Errorf("expected ID 123, got %v", pkt.C)
		}
		if pkt.P != "hello from client" {
			return fmt.Errorf("expected payload 'hello from client', got %v", pkt.P)
		}

		// 2. Write ControlPacket
		outID := 456
		outPayload := "hello from server"
		outPkt := ControlPacket[string]{
			C: &outID,
			P: outPayload,
		}
		if err := tr.WriteJSON(&outPkt); err != nil {
			return fmt.Errorf("write cp error: %w", err)
		}

		// 3. Read Regular Packet (ID should be dropped)
		var simple string
		if err := tr.ReadJSON(&simple); err != nil {
			return fmt.Errorf("read simple error: %w", err)
		}
		if simple != "ignore my id" {
			return fmt.Errorf("expected 'ignore my id', got %q", simple)
		}

		return nil
	}

	c := connForTest(t, SocketOpts{}, handler)

	// Handshake
	if err := wsjson.Write(t.Context(), c, map[string]string{"type": "hello", "version": "1"}); err != nil {
		t.Fatalf("handshake write failed: %v", err)
	}
	var resp HandshakeResponse
	if err := wsjson.Read(t.Context(), c, &resp); err != nil {
		t.Fatalf("handshake read failed: %v", err)
	}

	// 1. Client sends control packet manually
	// Format: 123:"hello from client"
	msg1 := []byte(`123:"hello from client"`)
	if err := c.Write(t.Context(), websocket.MessageText, msg1); err != nil {
		t.Fatalf("client write 1 failed: %v", err)
	}

	// 2. Client reads control packet manually
	// Expected: 456:"hello from server"
	typ, b, err := c.Read(t.Context())
	if err != nil {
		t.Fatalf("client read 1 failed: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("expected text message")
	}
	if string(b) != `456:"hello from server"` {
		t.Errorf("client read 1 mismatch. got %q", string(b))
	}

	// 3. Client sends control packet, server reads as normal struct
	// Format: 789:"ignore my id"
	msg2 := []byte(`789:"ignore my id"`)
	if err := c.Write(t.Context(), websocket.MessageText, msg2); err != nil {
		t.Fatalf("client write 2 failed: %v", err)
	}
}
