package transport

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSMuxClient(t *testing.T) {
	// 1. Setup Context and BufferPair
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := NewBufferPair(ctx, 16)

	// 2. Setup SMux Clients
	// Side 1 (Client initiator)
	c1, call1 := SMuxClient(t1, nil)

	// Side 2 (Server receiver)
	incomingCalls := make(chan Transport, 1)
	c2, _ := SMuxClient(t2, func(tr Transport) error {
		incomingCalls <- tr
		<-tr.Context().Done()
		return nil
	})

	// 3. Start Message Pumps (Top-level)
	// We need to consume messages from c1 and c2 to drive the multiplexing.
	// We also want to verify top-level messages.
	topLevel1 := make(chan json.RawMessage, 10)
	topLevel2 := make(chan json.RawMessage, 10)

	var wg sync.WaitGroup
	wg.Add(2)

	pump := func(tr Transport, out chan<- json.RawMessage) {
		defer wg.Done()
		for {
			var v json.RawMessage
			if err := tr.ReadJSON(&v); err != nil {
				return // Context canceled or closed
			}
			select {
			case out <- v:
			default:
				// buffer full, drop or panic in test
			}
		}
	}

	go pump(c1, topLevel1)
	go pump(c2, topLevel2)

	// 4. Test Top-Level Communication
	msg1 := "hello top level 1->2"
	if err := c1.WriteJSON(msg1); err != nil {
		t.Fatalf("c1.WriteJSON failed: %v", err)
	}

	select {
	case raw := <-topLevel2:
		var got string
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("failed to unmarshal top level msg: %v", err)
		}
		if got != msg1 {
			t.Errorf("top level 2 received %q, want %q", got, msg1)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for top level message on c2")
	}

	msg2 := "hello top level 2->1"
	if err := c2.WriteJSON(msg2); err != nil {
		t.Fatalf("c2.WriteJSON failed: %v", err)
	}

	select {
	case raw := <-topLevel1:
		var got string
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("failed to unmarshal top level msg: %v", err)
		}
		if got != msg2 {
			t.Errorf("top level 1 received %q, want %q", got, msg2)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for top level message on c1")
	}

	// 5. Test Sub-Channel (Call) Initiation
	arg := map[string]string{"foo": "bar"}
	subC1 := call1(ctx, arg)
	if subC1 == nil {
		t.Fatal("call1 returned nil transport")
	}

	var subC2 Transport
	select {
	case subC2 = <-incomingCalls:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for incoming call on c2")
	}

	// Verify Argument
	gotArg, ok := SMuxArg[map[string]string](subC2)
	if !ok {
		t.Fatalf("DecodeSMuxArg failed")
	}
	if !reflect.DeepEqual(gotArg, arg) {
		t.Errorf("received arg %v, want %v", gotArg, arg)
	}

	// 6. Test Sub-Channel Communication
	subMsg1 := "hello sub 1->2"
	if err := subC1.WriteJSON(subMsg1); err != nil {
		t.Fatalf("subC1.WriteJSON failed: %v", err)
	}

	var gotSubMsg1 string
	// subC2.ReadJSON should block until message arrives
	if err := subC2.ReadJSON(&gotSubMsg1); err != nil {
		t.Fatalf("subC2.ReadJSON failed: %v", err)
	}
	if gotSubMsg1 != subMsg1 {
		t.Errorf("subC2 received %q, want %q", gotSubMsg1, subMsg1)
	}

	subMsg2 := "hello sub 2->1"
	if err := subC2.WriteJSON(subMsg2); err != nil {
		t.Fatalf("subC2.WriteJSON failed: %v", err)
	}

	var gotSubMsg2 string
	if err := subC1.ReadJSON(&gotSubMsg2); err != nil {
		t.Fatalf("subC1.ReadJSON failed: %v", err)
	}
	if gotSubMsg2 != subMsg2 {
		t.Errorf("subC1 received %q, want %q", gotSubMsg2, subMsg2)
	}

	// 7. Test Parallel Sub-Channels (optional but good)
	// Let's open another call
	arg2 := "call2"
	subC1b := call1(ctx, arg2)

	var subC2b Transport
	select {
	case subC2b = <-incomingCalls:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for second call")
	}

	// Verify they are distinct
	if err := subC1b.WriteJSON("b"); err != nil {
		t.Fatal(err)
	}
	var gotB string
	if err := subC2b.ReadJSON(&gotB); err != nil {
		t.Fatal(err)
	}
	if gotB != "b" {
		t.Errorf("subC2b got %q want 'b'", gotB)
	}

	// Cleanup
	cancel()
	wg.Wait()
}

func TestSMuxStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := NewBufferPair(ctx, 16)

	// Side 1 (Client)
	c1, call1 := SMuxClient(t1, nil)

	// Side 2 (Server)
	incomingCalls := make(chan Transport, 1)
	c2, _ := SMuxClient(t2, func(tr Transport) error {
		incomingCalls <- tr
		<-tr.Context().Done()
		return nil
	})

	// Message Pumps
	go func() {
		for {
			var v any
			if err := c1.ReadJSON(&v); err != nil {
				return
			}
		}
	}()
	go func() {
		for {
			var v any
			if err := c2.ReadJSON(&v); err != nil {
				return
			}
		}
	}()

	// 1. Initiate call and cancel with specific error
	callCtx, callCancel := context.WithCancelCause(ctx)
	myErr := errors.New("my custom stop error")

	sub1 := call1(callCtx, "arg")
	var sub2 Transport
	select {
	case sub2 = <-incomingCalls:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// 2. Send some data then cancel
	if err := sub1.WriteJSON("hello"); err != nil {
		t.Fatal(err)
	}
	var msg string
	if err := sub2.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}

	callCancel(myErr)

	// 3. Side 2 should eventually see the error
	errCh := make(chan error, 1)
	go func() {
		var dummy any
		errCh <- sub2.ReadJSON(&dummy)
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != myErr.Error() {
			t.Errorf("got error %q, want %q", err.Error(), myErr.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for stop error propagation")
	}
}

func TestSMuxStopNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := NewBufferPair(ctx, 16)

	c1, call1 := SMuxClient(t1, nil)

	incomingCalls := make(chan Transport, 1)
	c2, _ := SMuxClient(t2, func(tr Transport) error {
		incomingCalls <- tr
		<-tr.Context().Done()
		return nil
	})

	go func() {
		for {
			var v any
			if err := c1.ReadJSON(&v); err != nil {
				return
			}
		}
	}()
	go func() {
		for {
			var v any
			if err := c2.ReadJSON(&v); err != nil {
				return
			}
		}
	}()

	callCtx, callCancel := context.WithCancelCause(ctx)
	sub1 := call1(callCtx, "arg")
	var sub2 Transport
	select {
	case sub2 = <-incomingCalls:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	if err := sub1.WriteJSON("hello"); err != nil {
		t.Fatal(err)
	}
	var msg string
	if err := sub2.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}

	callCancel(nil)

	errCh := make(chan error, 1)
	go func() {
		var dummy any
		errCh <- sub2.ReadJSON(&dummy)
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got error %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
