# Loom

Loom is a lightweight streaming queue/router for **large messages** (e.g. 1–256MiB) over **QUIC** or **HTTP/3**.
It is designed for “Kafka/MQ-like” partition routing, but optimized for **big binary payloads** by chunking and applying backpressure.

## What it does

- Producers stream a message as: `message header (routing key + optional declared size)` + `N chunks` + `end-of-message`.
- The server routes each message to a consumer chosen by **partitioned rendezvous hashing** of the routing key.
- Limits and behavior are controlled by `loom.yaml`:
  - `router.max_message_bytes` (default 256MiB)
  - `router.max_chunk_bytes` (default 64KiB)
  - backlog and backpressure behavior for full partitions / chunk pressure
  - per-message chunk buffering is derived from `max_chunk_bytes` (target ~1MiB)

## Run with Docker

```bash
docker build -t loom:dev .

docker run --rm \
  -p 4242:4242/udp \
  -p 9090:9090 \
  -v "$PWD/loom.yaml:/config/loom.yaml:ro" \
  loom:dev /config/loom.yaml
```

## Clients

Implement the Loom wire protocol over QUIC (or HTTP/3). See `PROTOCOL.md`.

## Notes

- This is a **streaming** router, not a durable log; if no consumers are connected, messages are discarded.
- For production, configure TLS cert/key and set `insecure_skip_verify: false` for clients.
- Yes I wrote this README with ChatGPT. Bite me.
