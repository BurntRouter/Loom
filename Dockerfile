# Build: docker build -t loom .
# Run:   docker run --rm -p 4242:4242/udp -p 9090:9090 -v $PWD/loom.yaml:/config/loom.yaml:ro loom /config/loom.yaml

FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/loom ./cmd/loomd

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/loom /loom

EXPOSE 4242/udp
EXPOSE 9090/tcp

ENTRYPOINT ["/loom"]
CMD ["/config/loom.yaml"]
