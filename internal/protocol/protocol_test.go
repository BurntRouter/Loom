package protocol

import (
	"bufio"
	"bytes"
	"testing"
)

func TestHelloRoundTrip(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	if err := WriteHello(w, RoleProducer, "p1", "room", "tok"); err != nil {
		t.Fatal(err)
	}

	r := bufio.NewReader(bytes.NewReader(b.Bytes()))
	role, name, room, token, err := ReadHello(r, 32, 32, 32)
	if err != nil {
		t.Fatal(err)
	}
	if role != RoleProducer || name != "p1" || room != "room" || token != "tok" {
		t.Fatalf("unexpected hello: role=%c name=%q room=%q token=%q", role, name, room, token)
	}
}

func TestMessageHeaderChunkAckRoundTrip(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	if err := WriteMessageHeader(w, []byte("k"), 123, 42); err != nil {
		t.Fatal(err)
	}
	if err := WriteChunk(w, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := WriteEndOfMessage(w); err != nil {
		t.Fatal(err)
	}
	if err := WriteAck(w, 42); err != nil {
		t.Fatal(err)
	}

	r := bufio.NewReader(bytes.NewReader(b.Bytes()))
	h, err := ReadMessageHeader(r, 8)
	if err != nil {
		t.Fatal(err)
	}
	if string(h.Key) != "k" || h.DeclaredSize != 123 || h.MsgID != 42 {
		t.Fatalf("unexpected header: %+v", h)
	}
	chunk, done, err := ReadChunk(r, 16)
	if err != nil {
		t.Fatal(err)
	}
	if done || string(chunk) != "abc" {
		t.Fatalf("unexpected chunk: done=%v chunk=%q", done, string(chunk))
	}
	_, done, err = ReadChunk(r, 16)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected end-of-message")
	}
	ft, mid, err := ReadFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if ft != FrameAck || mid != 42 {
		t.Fatalf("unexpected frame: type=%d msgID=%d", ft, mid)
	}
}
