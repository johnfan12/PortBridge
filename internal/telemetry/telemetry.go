package telemetry

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"
)

type PortStatus struct {
	Port  int  `json:"port"`
	Ready bool `json:"ready"`
}
type Status struct {
	TCPEnabled bool         `json:"tcp_enabled"`
	TCPPaused  bool         `json:"tcp_paused"`
	TCPPorts   []PortStatus `json:"tcp_ports"`
	Hostname   string       `json:"hostname"`
	At         time.Time    `json:"at"`
}

// Probe only explicitly authorized ports, with a shared half-second time bound.
func ProbePorts(ctx context.Context, ports []int) []PortStatus {
	result := make([]PortStatus, len(ports))
	var wg sync.WaitGroup
	for i, port := range ports {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result[i].Port = port
			probe, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()
			conn, err := (&net.Dialer{}).DialContext(probe, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err == nil {
				result[i].Ready = true
				conn.Close()
			}
		}()
	}
	wg.Wait()
	return result
}
