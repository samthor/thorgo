package transport

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestSMux(t *testing.T) {
	type MyArg struct {
		Name string `json:"name"`
	}

	// Server side
	handler := func(tr Transport) error {
		top := SMux(tr, func(call Transport, arg MyArg) error {
			// Sub-transport handler
			var msg string
			if err := call.ReadJSON(&msg); err != nil {
				return err
			}
			return call.WriteJSON(fmt.Sprintf("hello %s, you said %s", arg.Name, msg))
		})

		// Drive SMux by reading from top-level
		for {
			var msg string
			if err := top.ReadJSON(&msg); err != nil {
				return err
			}
			if err := top.WriteJSON("echo:" + msg); err != nil {
				return err
			}
		}
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

	// 1. Test top-level communication
	if err := wsjson.Write(t.Context(), c, "top1"); err != nil {
		t.Fatalf("write top1 failed: %v", err)
	}
	var out string
	if err := wsjson.Read(t.Context(), c, &out); err != nil {
		t.Fatalf("read top1 failed: %v", err)
	}
	if out != "echo:top1" {
		t.Errorf("got %q, want echo:top1", out)
	}

	// 2. Start a call
	// :{"id": 1, "arg": {"name": "world"}}
	startCall := []byte(`:{"id": 1, "arg": {"name": "world"}}`)
	if err := c.Write(t.Context(), websocket.MessageText, startCall); err != nil {
		t.Fatalf("write startCall failed: %v", err)
	}

	// 3. Send message to call 1
	// The lastIncomingID is now 1 on server.
	if err := wsjson.Write(t.Context(), c, "call-msg"); err != nil {
		t.Fatalf("write call-msg failed: %v", err)
	}

	// 4. Expect response from call 1
	// The server will send:
	// :{"id": 1}
	// "hello world, you said call-msg"
	// :{"stop": ""} (because the handler returns nil)

	// Read :{"id": 1}
	_, b, err := c.Read(t.Context())
	if err != nil {
		t.Fatalf("read id:1 failed: %v", err)
	}
	if string(b) != `:{"id":1}` {
		t.Errorf("expected id:1 control, got %q", string(b))
	}

	// Read payload
	_, b, err = c.Read(t.Context())
	if err != nil {
		t.Fatalf("read call payload failed: %v", err)
	}
	var callPayload string
	if err := json.Unmarshal(b, &callPayload); err != nil {
		t.Fatalf("unmarshal call payload failed: %v, raw: %q", err, string(b))
	}
	expectedPayload := "hello world, you said call-msg"
	if callPayload != expectedPayload {
		t.Errorf("expected call payload %q, got %q", expectedPayload, callPayload)
	}

	// Read :{"id":1,"stop":""}
	_, b, err = c.Read(t.Context())
	if err != nil {
		t.Fatalf("read stop failed: %v", err)
	}
	if b[0] != ':' {
		t.Fatalf("expected ':' prefix for stop control, got %q", string(b))
	}
	payloadBytes := b[1:]
	var stopPkt struct {
		ID   int    `json:"id"`
		Stop string `json:"stop"`
	}
	if err := json.Unmarshal(payloadBytes, &stopPkt); err != nil {
		t.Fatalf("unmarshal stop failed: %v, raw: %q", err, string(payloadBytes))
	}
	if stopPkt.ID != 1 {
		t.Errorf("expected stop for ID 1, got %d", stopPkt.ID)
	}
	// The stop message is empty because the handler returned nil
	if stopPkt.Stop != "" {
		t.Errorf("expected empty stop reason, got %q", stopPkt.Stop)
	}

	// 5. Test top-level again
	// Need to reset lastIncomingID to 0
	resetID := []byte(`:{"id": 0}`)
	if err := c.Write(t.Context(), websocket.MessageText, resetID); err != nil {
		t.Fatalf("write resetID failed: %v", err)
	}
	if err := wsjson.Write(t.Context(), c, "top2"); err != nil {
		t.Fatalf("write top2 failed: %v", err)
	}

	// Server should send :{"id":0} before sending "echo:top2"
	_, b, err = c.Read(t.Context())
	if err != nil {
		t.Fatalf("read id:0 failed: %v", err)
	}
	if string(b) != `:{"id":0}` {
		t.Errorf("expected id:0 control, got %q", string(b))
	}

	_, b, err = c.Read(t.Context())
	if err != nil {
		t.Fatalf("read top2 failed: %v", err)
	}
	var top2Payload string
	if err := json.Unmarshal(b, &top2Payload); err != nil {
		t.Fatalf("unmarshal top2 failed: %v, raw: %q", err, string(b))
	}
	if top2Payload != "echo:top2" {
		t.Errorf("expected echo:top2, got %q", top2Payload)
	}
}
