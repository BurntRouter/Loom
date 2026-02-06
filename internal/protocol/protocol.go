package protocol

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	Magic       = "LOOM"
	VersionByte = 4

	FrameAck = uint64(1)

	RoleProducer = byte('P')
	RoleConsumer = byte('C')
)

var ErrBadHandshake = errors.New("protocol: bad handshake")

// WriteHello writes the Loom stream preface.
func WriteHello(w *bufio.Writer, role byte, name, room, token string) error {
	if role != RoleProducer && role != RoleConsumer {
		return fmt.Errorf("protocol: unknown role %q", role)
	}
	if _, err := w.WriteString(Magic); err != nil {
		return err
	}
	if err := w.WriteByte(VersionByte); err != nil {
		return err
	}
	if err := w.WriteByte(role); err != nil {
		return err
	}
	if err := writeUvarint(w, uint64(len(name))); err != nil {
		return err
	}
	if _, err := w.WriteString(name); err != nil {
		return err
	}
	if err := writeUvarint(w, uint64(len(room))); err != nil {
		return err
	}
	if _, err := w.WriteString(room); err != nil {
		return err
	}
	if err := writeUvarint(w, uint64(len(token))); err != nil {
		return err
	}
	if _, err := w.WriteString(token); err != nil {
		return err
	}
	return w.Flush()
}

// ReadHello reads the Loom stream preface.
func ReadHello(r *bufio.Reader, maxNameBytes, maxRoomBytes, maxTokenBytes int) (role byte, name, room, token string, err error) {
	var preface [len(Magic) + 2]byte
	if _, err := io.ReadFull(r, preface[:]); err != nil {
		return 0, "", "", "", err
	}
	if string(preface[:len(Magic)]) != Magic {
		return 0, "", "", "", ErrBadHandshake
	}
	if preface[len(Magic)] != VersionByte {
		return 0, "", "", "", ErrBadHandshake
	}
	role = preface[len(Magic)+1]
	nameLen, err := readUvarint(r)
	if err != nil {
		return 0, "", "", "", err
	}
	if nameLen > uint64(maxNameBytes) {
		return 0, "", "", "", fmt.Errorf("protocol: name too large: %d", nameLen)
	}
	nameBuf := make([]byte, int(nameLen))
	if _, err := io.ReadFull(r, nameBuf); err != nil {
		return 0, "", "", "", err
	}

	roomLen, err := readUvarint(r)
	if err != nil {
		return 0, "", "", "", err
	}
	if roomLen > uint64(maxRoomBytes) {
		return 0, "", "", "", fmt.Errorf("protocol: room too large: %d", roomLen)
	}
	roomBuf := make([]byte, int(roomLen))
	if _, err := io.ReadFull(r, roomBuf); err != nil {
		return 0, "", "", "", err
	}

	tokLen, err := readUvarint(r)
	if err != nil {
		return 0, "", "", "", err
	}
	if tokLen > uint64(maxTokenBytes) {
		return 0, "", "", "", fmt.Errorf("protocol: token too large: %d", tokLen)
	}
	tokBuf := make([]byte, int(tokLen))
	if _, err := io.ReadFull(r, tokBuf); err != nil {
		return 0, "", "", "", err
	}

	return role, string(nameBuf), string(roomBuf), string(tokBuf), nil
}

type MessageHeader struct {
	Key          []byte
	DeclaredSize uint64
	MsgID        uint64
}

func ReadMessageHeader(r *bufio.Reader, maxKeyBytes int) (MessageHeader, error) {
	keyLen, err := readUvarint(r)
	if err != nil {
		return MessageHeader{}, err
	}
	if keyLen == 0 {
		return MessageHeader{}, errors.New("protocol: empty key - possible stream corruption")
	}
	if keyLen > uint64(maxKeyBytes) {
		return MessageHeader{}, fmt.Errorf("protocol: key too large: %d (max %d) - stream likely corrupted", keyLen, maxKeyBytes)
	}
	key := make([]byte, int(keyLen))
	if _, err := io.ReadFull(r, key); err != nil {
		return MessageHeader{}, err
	}
	sz, err := readUvarint(r)
	if err != nil {
		return MessageHeader{}, err
	}
	msgID, err := readUvarint(r)
	if err != nil {
		return MessageHeader{}, err
	}
	return MessageHeader{Key: key, DeclaredSize: sz, MsgID: msgID}, nil
}

func WriteMessageHeader(w *bufio.Writer, key []byte, declaredSize uint64, msgID uint64) error {
	if len(key) == 0 {
		return errors.New("protocol: empty key")
	}
	if err := writeUvarint(w, uint64(len(key))); err != nil {
		return err
	}
	if _, err := w.Write(key); err != nil {
		return err
	}
	if err := writeUvarint(w, declaredSize); err != nil {
		return err
	}
	if err := writeUvarint(w, msgID); err != nil {
		return err
	}
	return nil
}

// ReadChunk reads a single chunk. A nil slice with done=true indicates end-of-message.
func ReadChunk(r *bufio.Reader, maxChunkBytes int) (chunk []byte, done bool, err error) {
	n, err := readUvarint(r)
	if err != nil {
		return nil, false, err
	}
	if n == 0 {
		return nil, true, nil
	}
	if n > uint64(maxChunkBytes) {
		return nil, false, fmt.Errorf("protocol: chunk too large: %d (max %d) - stream likely corrupted", n, maxChunkBytes)
	}
	buf := make([]byte, int(n))
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, false, err
	}
	return buf, false, nil
}

func WriteChunk(w *bufio.Writer, chunk []byte) error {
	if err := writeUvarint(w, uint64(len(chunk))); err != nil {
		return err
	}
	if len(chunk) == 0 {
		return nil
	}
	_, err := w.Write(chunk)
	return err
}

func WriteEndOfMessage(w *bufio.Writer) error {
	return writeUvarint(w, 0)
}

func DiscardMessage(r *bufio.Reader, maxChunkBytes int) error {
	for {
		n, err := readUvarint(r)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if n > uint64(maxChunkBytes) {
			return fmt.Errorf("protocol: chunk too large: %d (max %d) - stream corrupted, cannot skip", n, maxChunkBytes)
		}
		if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
			return err
		}
	}
}

func writeUvarint(w io.Writer, v uint64) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	_, err := w.Write(buf[:n])
	return err
}

func readUvarint(r *bufio.Reader) (uint64, error) {
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	// Sanity check: varints larger than 1PB (2^50) are almost certainly corruption
	if v > 1<<50 {
		return 0, fmt.Errorf("protocol: invalid varint value %d - stream corrupted", v)
	}
	return v, nil
}

func WriteAck(w *bufio.Writer, msgID uint64) error {
	if err := writeUvarint(w, FrameAck); err != nil {
		return err
	}
	if err := writeUvarint(w, msgID); err != nil {
		return err
	}
	return w.Flush()
}

func ReadFrame(r *bufio.Reader) (frameType uint64, msgID uint64, err error) {
	ft, err := readUvarint(r)
	if err != nil {
		return 0, 0, err
	}
	mid, err := readUvarint(r)
	if err != nil {
		return 0, 0, err
	}
	return ft, mid, nil
}

// EncodeHello is a convenience for tests and debugging.
func EncodeHello(role byte, name, room, token string) []byte {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	_ = WriteHello(w, role, name, room, token)
	return buf.Bytes()
}
