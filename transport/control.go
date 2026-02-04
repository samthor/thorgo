package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// ControlPacket wraps an optional control ID and an actual data packet.
// It has special encoding over a WebSocket, but is otherwise "just a tuple".
type ControlPacket[Type any] struct {
	C *int `json:"c,omitzero"`
	P Type `json:"p,omitzero"`
}

type socketControlPacket interface {
	update(cp ControlPacket[json.RawMessage]) (err error)
	socketEncode() (b []byte, err error)
	socketDecode(b []byte) (err error)
}

func (cp *ControlPacket[Type]) update(from ControlPacket[json.RawMessage]) (err error) {
	err = json.Unmarshal(from.P, &cp.P)
	if err != nil {
		return err
	}
	cp.C = from.C
	return nil
}

func (cp *ControlPacket[Type]) socketEncode() (b []byte, err error) {
	b, err = json.Marshal(cp.P)
	if err != nil || cp.C == nil {
		return
	}

	prefix := []byte(":")
	if *cp.C != 0 {
		prefix = []byte(fmt.Sprintf("%d:", *cp.C))
	}

	b = append(prefix, b...)
	return b, nil
}

func (cp *ControlPacket[Type]) socketDecode(b []byte) (err error) {
	if len(b) != 0 {
		var id int64
		var hasID bool

		if b[0] == ':' {
			b = b[1:]
			hasID = true
		} else if b[0] == '-' || (b[0] >= '0' && b[0] <= '9') {
			// look for ":"
			index := bytes.IndexByte(b, ':')
			if index != -1 {
				id, err = strconv.ParseInt(string(b[:index]), 10, 32)
				if err != nil {
					return
				}
				b = b[index+1:]
				hasID = true
			}
		}

		if hasID {
			intID := int(id)
			cp.C = &intID
		}
	}

	return json.Unmarshal(b, &cp.P)
}

func controlDecode(raw []byte, v any) (err error) {
	var dec ControlPacket[json.RawMessage]
	err = dec.socketDecode(raw)
	if err != nil {
		return err
	}

	if cp, ok := v.(socketControlPacket); ok {
		return cp.update(dec)
	}
	return json.Unmarshal(dec.P, v)
}

func controlEncode(v any) (b []byte, err error) {
	if cp, ok := v.(socketControlPacket); ok {
		return cp.socketEncode()
	}
	return json.Marshal(v)
}
