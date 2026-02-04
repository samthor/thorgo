package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSMuxFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, wire := NewBufferPair(ctx, 16)

	type MyArg struct {
		Name string `json:"name"`
	}

	type transportPacket struct {
		Control bool            `json:"control"`
		Data    json.RawMessage `json:"data"`
	}

	// Server side (the SMux wrapper)
	handler := func(tr Transport) error {
		top := SMux(tr, func(call Transport) error {
			var arg MyArg
			if err := DecodeSMuxArg(call, &arg); err != nil {
				return err
			}
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

	// Run the server in a goroutine
	go func() {
		handler(client)
	}()

	// 1. Test top-level communication (Control=false)
	// Write: transportPacket{Control: false, Data: "top1"}
	// "top1" json marshaled is "\"top1\""
	tp1 := transportPacket{
		Control: false,
		Data:    json.RawMessage(`"top1"`),
	}
	if err := wire.WriteJSON(tp1); err != nil {
		t.Fatalf("write top1 failed: %v", err)
	}

	// Read response
	var resp1 transportPacket
	if err := wire.ReadJSON(&resp1); err != nil {
		t.Fatalf("read top1 failed: %v", err)
	}
	if resp1.Control {
		t.Errorf("expected non-control response for top-level")
	}
	var echo1 string
	if err := json.Unmarshal(resp1.Data, &echo1); err != nil {
		t.Fatalf("unmarshal echo1 failed: %v", err)
	}
	if echo1 != "echo:top1" {
		t.Errorf("got %q, want echo:top1", echo1)
	}

	// 2. Start a call (Control=true)
	// Send: transportPacket{Control: true, Data: ...}
	// Data: {"id": 1, "arg": {"name": "world"}}
	startCallJSON := `{"id": 1, "arg": {"name": "world"}}`
	tpStart := transportPacket{
		Control: true,
		Data:    json.RawMessage(startCallJSON),
	}
	if err := wire.WriteJSON(tpStart); err != nil {
		t.Fatalf("write startCall failed: %v", err)
	}

	// 3. Send message to call 1 (Control=false)
	// Since fallback doesn't use ID-based multiplexing on the wire for data packets (it relies on SMux internal state for the *lastIncomingID*),
	// wait, SMux fallback sends everything as `transportPacket`.
	// For DATA packets sent TO the SMux, `processPacket` uses `lastIncomingID`.
	// So we need to ensure SMux knows which call we are talking to.
	// `processControl` sets `s.lastIncomingID = smuxControl.ID`.
	// So the previous startCall set `lastIncomingID` to 1.

	// Send "call-msg"
	tpMsg := transportPacket{
		Control: false,
		Data:    json.RawMessage(`"call-msg"`),
	}
	if err := wire.WriteJSON(tpMsg); err != nil {
		t.Fatalf("write call-msg failed: %v", err)
	}

	// 4. Expect response from call 1
	// The server will send control packet to set ID if it changed?
	// `internalSend` checks `s.lastOutgoingID != id`.
	// If it's different, it sends a control packet `{id: ...}`.
	// Initially `lastOutgoingID` is 0.
	// We are sending from call 1. So server should send `Control=true, Data={"id":1}` first.

	var resp2 transportPacket
	if err := wire.ReadJSON(&resp2); err != nil {
		t.Fatalf("read id:1 failed: %v", err)
	}
	if !resp2.Control {
		t.Errorf("expected control packet for ID switch")
	}
	idSwitch := struct {
		ID int `json:"id"`
	}{}
	if err := json.Unmarshal(resp2.Data, &idSwitch); err != nil {
		t.Fatalf("unmarshal id switch failed: %v", err)
	}
	if idSwitch.ID != 1 {
		t.Errorf("expected switch to ID 1, got %d", idSwitch.ID)
	}

	// Next packet: The actual data "hello world, you said call-msg"
	// This will be `Control=false`.
	var resp3 transportPacket
	if err := wire.ReadJSON(&resp3); err != nil {
		t.Fatalf("read payload failed: %v", err)
	}
	if resp3.Control {
		t.Errorf("expected data packet, got control")
	}
	var callPayload string
	if err := json.Unmarshal(resp3.Data, &callPayload); err != nil {
		t.Fatalf("unmarshal call payload failed: %v", err)
	}
	expected := "hello world, you said call-msg"
	if callPayload != expected {
		t.Errorf("got %q, want %q", callPayload, expected)
	}

	// Next packet: Call finishes, so it sends stop.
	// `internalSend` with stop=true sends `Control=true` and `stop` field.
	var resp4 transportPacket
	if err := wire.ReadJSON(&resp4); err != nil {
		t.Fatalf("read stop failed: %v", err)
	}
	if !resp4.Control {
		t.Errorf("expected control packet for stop")
	}
	// The struct used in internalSend for stop is: struct { ID int; Stop string }
	var stopPkt struct {
		ID   int     `json:"id"`
		Stop *string `json:"stop"`
	}
	if err := json.Unmarshal(resp4.Data, &stopPkt); err != nil {
		t.Fatalf("unmarshal stop failed: %v", err)
	}
	if stopPkt.ID != 1 {
		t.Errorf("expected stop for ID 1, got %d", stopPkt.ID)
	}
	if stopPkt.Stop == nil {
		t.Errorf("expected stop field")
	} else if *stopPkt.Stop != "" {
		t.Errorf("expected empty stop reason, got %q", *stopPkt.Stop)
	}

	// 5. Test top-level again
	// We need to switch back to ID 0 manually on the client side if we want to talk to top-level?
	// `SMux` processControl handles `ID: 0` to reset `lastIncomingID`.
	// Send: Control=true, Data={"id": 0}
	tpReset := transportPacket{
		Control: true,
		Data:    json.RawMessage(`{"id": 0}`),
	}
	if err := wire.WriteJSON(tpReset); err != nil {
		t.Fatalf("write reset failed: %v", err)
	}

	// Send top-level message
	tp2 := transportPacket{
		Control: false,
		Data:    json.RawMessage(`"top2"`),
	}
	if err := wire.WriteJSON(tp2); err != nil {
		t.Fatalf("write top2 failed: %v", err)
	}

	// Server should send ID switch to 0 first (since it was 1)
	var resp5 transportPacket
	if err := wire.ReadJSON(&resp5); err != nil {
		t.Fatalf("read id:0 failed: %v", err)
	}
	if !resp5.Control {
		t.Errorf("expected control packet for ID switch back to 0")
	}
	if err := json.Unmarshal(resp5.Data, &idSwitch); err != nil {
		t.Fatalf("unmarshal id switch 0 failed: %v", err)
	}
	if idSwitch.ID != 0 {
		t.Errorf("expected switch to ID 0, got %d", idSwitch.ID)
	}

	// Server echoes "top2"
	var resp6 transportPacket
	if err := wire.ReadJSON(&resp6); err != nil {
		t.Fatalf("read top2 failed: %v", err)
	}
	if resp6.Control {
		t.Errorf("expected data packet for top2")
	}
	var echo2 string
	if err := json.Unmarshal(resp6.Data, &echo2); err != nil {
		t.Fatalf("unmarshal echo2 failed: %v", err)
	}
	if echo2 != "echo:top2" {
		t.Errorf("got %q, want echo:top2", echo2)
	}
}
