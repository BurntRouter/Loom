package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	Connections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "loom_connections", Help: "Active connections"},
		[]string{"transport"},
	)
	Streams = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "loom_streams", Help: "Active streams"},
		[]string{"transport", "role", "room"},
	)
	MessagesIn = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "loom_messages_in_total", Help: "Messages received from producers"},
		[]string{"room"},
	)
	MessagesOut = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "loom_messages_out_total", Help: "Messages routed to consumers"},
		[]string{"room"},
	)
	BytesIn = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "loom_bytes_in_total", Help: "Payload bytes received from producers"},
		[]string{"room"},
	)
	BytesOut = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "loom_bytes_out_total", Help: "Payload bytes written to consumers"},
		[]string{"room"},
	)
	Drops = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "loom_drops_total", Help: "Dropped messages"},
		[]string{"room", "reason"},
	)
	ProtocolErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "loom_protocol_errors_total", Help: "Protocol errors from producers"},
		[]string{"room", "error_type"},
	)
	BlockedProducers = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "loom_blocked_producers_total", Help: "Producers blocked due to repeated errors"},
		[]string{"room"},
	)
)

func Register() {
	prometheus.MustRegister(Connections, Streams, MessagesIn, MessagesOut, BytesIn, BytesOut, Drops, ProtocolErrors, BlockedProducers)
}
