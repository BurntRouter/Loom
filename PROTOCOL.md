# Loom Wire Protocol (v4)

Loom uses a simple framed binary protocol over a reliable byte stream.
Today this stream is carried over:
- **QUIC** (raw QUIC stream)
- **HTTP/3** (POST `/stream`, request body is the stream; server streams responses for consumers)

## Conventions

- Varints are **unsigned binary varints** as implemented by Go’s `encoding/binary`:
  - `PutUvarint` / `ReadUvarint`
- All strings/byte slices are length-prefixed with a uvarint.

## Stream Preface (Hello)

Immediately upon opening a stream, the client sends:

1. ASCII magic: `"LOOM"` (4 bytes)
2. Version: `0x04` (1 byte)
3. Role: one byte
   - `P` (`0x50`) producer
   - `C` (`0x43`) consumer
4. `name_len` (uvarint) + `name` (bytes)
5. `room_len` (uvarint) + `room` (bytes)
6. `token_len` (uvarint) + `token` (bytes)

## Producer → Server Messages

After Hello, the producer sends **one or more messages**:

### Message header

- `key_len` (uvarint) + `key` (bytes)
- `declared_size` (uvarint) (0 means “unknown”)
- `msg_id` (uvarint) (client may send 0; server assigns and forwards its own id)

### Message body (chunks)

Repeated:
- `chunk_len` (uvarint)
  - if `chunk_len == 0`: end-of-message
  - else: `chunk_bytes` (exactly `chunk_len` bytes)

## Server → Consumer Messages

Consumers receive the same framing for each routed message:

- `key_len` + `key`
- `declared_size`
- `msg_id`
- chunks until `chunk_len == 0`

After fully processing a message, the consumer MUST ACK it on the same stream:

- `frame_type` (uvarint) = `1` (ACK)
- `msg_id` (uvarint)

## Limits / Behavior

- The server enforces `router.max_chunk_bytes` and `router.max_message_bytes`.
- `router.max_backlog_depth` is a per-consumer message backlog bound.
- Per-message chunk buffering is derived from `max_chunk_bytes` (target ~1MiB).

## Notes
- I also wrote this with ChatGPT. Bite me again.
