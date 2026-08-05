// Package session reads the sessions that the host holds now and reports the path of
// each one.
//
// The console warns the operator before a staged edit removes the path that carries the
// session of the operator. The console request itself states nothing, because
// internal/api/console.go refuses a console bind address that is not a loopback address.
// A warning that reads the path of the console request never fires. The operator reaches
// this host through a tailnet with another program, such as a shell session, therefore
// the daemon reads the sessions of the whole host.
//
// The package reports an inbound session only. A local rule of the chain HYDRASCALE-OUT
// governs the traffic that a namespace sends to the host, and the compiler writes no rule
// for a source that is the host, therefore an edit cuts an inbound session alone.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hydrascale/internal/access"
	"hydrascale/internal/execx"
)

// commandTimeout is the time that one command to the host gets. A command that does not
// return blocks the route that reads it, so every command carries this deadline.
const commandTimeout = 5 * time.Second

// Path is one endpoint pair of the local rule model that carries an active session.
// From names a tailnet and To holds the literal host.
type Path struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Reader reports the paths that carry an active session.
type Reader struct{ runner execx.Runner }

// NewReader returns a Reader that runs its commands on the host.
func NewReader() *Reader { return &Reader{runner: execx.OSRunner{}} }

// NewReaderWith returns a Reader that runs its commands through runner.
func NewReaderWith(runner execx.Runner) *Reader { return &Reader{runner: runner} }

// Paths returns one path for each tailnet that carries an active session, without a
// repeat.
//
// devices maps a tailnet identifier to the host side veth device of its namespace, which
// access.Topology holds. Paths reads the sessions of the host with `ss -H -tna`, and it
// asks the kernel for the device of each remote address with `ip -json route get`. A
// session whose device is the veth device of a tailnet carries the path from that tailnet
// to the host.
//
// Paths returns an error when a command fails and when the kernel answers with a route
// that it cannot read. It returns no path together with that error.
func (r *Reader) Paths(ctx context.Context, devices map[string]string) ([]Path, error) {
	rows, err := r.sessions(ctx)
	if err != nil {
		return nil, err
	}

	byDevice := make(map[string]string, len(devices))
	for id, device := range devices {
		byDevice[device] = id
	}

	listening := make(map[string]bool)
	for _, row := range rows {
		if row.state == "LISTEN" {
			listening[row.localPort] = true
		}
	}

	// The kernel returns the same device for the same address, therefore the reader asks
	// once for each address rather than once for each session.
	seen := make(map[string]bool)
	var paths []Path
	for _, row := range rows {
		// A session that arrives on a listening port comes from the peer. The rest of
		// the sessions leave the host, and no local rule governs them.
		if row.state != "ESTAB" || !listening[row.localPort] || seen[row.remoteHost] {
			continue
		}
		seen[row.remoteHost] = true

		device, err := r.device(ctx, row.remoteHost)
		if err != nil {
			return nil, err
		}
		id, ok := byDevice[device]
		if !ok {
			continue
		}
		paths = append(paths, Path{From: id, To: access.Host})
	}

	return withoutRepeat(paths), nil
}

// row is one line of `ss -H -tna`.
type row struct {
	state      string
	localPort  string
	remoteHost string
}

// sessions returns one row for each line that `ss -H -tna` wrote.
// sessions returns an error when the command fails.
func (r *Reader) sessions(ctx context.Context) ([]row, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	out, err := r.runner.Run(ctx, "ss", "-H", "-tna")
	if err != nil {
		return nil, fmt.Errorf("failed to read the sessions of the host with ss: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var rows []row
	for _, line := range strings.Split(string(out), "\n") {
		// The line holds the state, the receive queue, the send queue, the local address,
		// and the remote address, in that order.
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		_, localPort, ok := splitAddress(fields[3])
		if !ok {
			continue
		}
		remoteHost, _, ok := splitAddress(fields[4])
		if !ok {
			continue
		}
		rows = append(rows, row{state: fields[0], localPort: localPort, remoteHost: remoteHost})
	}
	return rows, nil
}

// splitAddress returns the host and the port of one address that ss wrote, and it reports
// whether the address holds both.
//
// ss writes an IPv4 address as 127.0.0.53:53, an IPv6 address as [::]:22, and it appends
// the zone of a link-local address as 127.0.0.53%lo. The port is the text after the last
// colon, because an IPv6 address holds a colon itself.
func splitAddress(address string) (host, port string, ok bool) {
	at := strings.LastIndex(address, ":")
	if at < 0 {
		return "", "", false
	}
	host, port = address[:at], address[at+1:]
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if at := strings.Index(host, "%"); at >= 0 {
		host = host[:at]
	}
	if host == "" || port == "" {
		return "", "", false
	}
	return host, port, true
}

// routeAnswer is one entry of the JSON array that `ip -json route get` writes.
type routeAnswer struct {
	Dev string `json:"dev"`
}

// device returns the name of the device that carries the traffic to address.
// device returns an error when the command fails, when the answer is not JSON, and when
// the kernel returns no entry.
func (r *Reader) device(ctx context.Context, address string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	out, err := r.runner.Run(ctx, "ip", "-json", "route", "get", address)
	if err != nil {
		return "", fmt.Errorf("failed to read the route to %s: %w: %s", address, err, strings.TrimSpace(string(out)))
	}

	var answers []routeAnswer
	if err := json.Unmarshal(out, &answers); err != nil {
		return "", fmt.Errorf("failed to read the route to %s: %w", address, err)
	}
	if len(answers) == 0 {
		return "", fmt.Errorf("the kernel returned no route to %s", address)
	}
	return answers[0].Dev, nil
}

// withoutRepeat returns the paths in the order that they arrived, with one entry for each
// pair. It returns an empty list rather than nil, because the console reads the field as
// a list.
func withoutRepeat(paths []Path) []Path {
	out := make([]Path, 0, len(paths))
	held := make(map[Path]bool, len(paths))
	for _, path := range paths {
		if held[path] {
			continue
		}
		held[path] = true
		out = append(out, path)
	}
	return out
}
