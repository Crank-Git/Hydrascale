# Hydrascale

Hydrascale is a Linux-only Go service that lets a single user run multiple Tailscale tailnets simultaneously. It achieves isolation by creating one network namespace per tailnet and launching a dedicated `tailscaled` instance inside that namespace. A unified DNS resolver aggregates all tailnet DNS servers, and routes are synced via `netlink`.

## Features

- Run multiple isolated Tailscale tailnets on a single machine
- Network namespace-based isolation for security and separation
- Unified DNS resolver for seamless name resolution across all tailnets
- Automatic route synchronization using netlink
- Systemd service integration for easy deployment
- CLI interface for managing tailnets
- Automatic restart of crashed daemons

## Architecture

```
┌───────────────────────┐
│   hydrascale CLI      │  (Cobra)
└────────▲───────────────┘
         │
add/remove/list     ──►  Service (systemd)
         │                     │
         ▼                     ▼
┌───────────────────────┐  ┌──────────────────────────┐
│ Namespace Manager     │  │ Tailscale Daemon Launcher│
│ (ip netns)            │  │   (tailscaled per ns)     │
└────────▲───────────────┘  └──────────────▲─────────────┘
         │                            │
         ▼                            ▼
┌───────────────────────┐  ┌──────────────────────────┐
│ Routing & Policy Engine │ │ DNS Unified Resolver     │
│ (netlink, route sync)  │ │ (UDP/TCP forwarder)     │
└────────▲───────────────┘  └──────────────────────────┘
         │                            │
         ▼                            ▼
Namespace‑specific routes & DNS  |  Global /etc/resolv.conf (optional)
```

## Installation

### Prerequisites

- Linux operating system (network namespaces require Linux)
- Tailscale installed (`tailscaled` and `tailscale` commands available)
- Root privileges or CAP_NET_ADMIN capability
- Go 1.22+ (for building from source)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/yourorg/hydrascale.git
cd hydrascale

# Build the binary
go build -o hydrascale ./cmd/hydrascale

# Install to /usr/local/bin (requires sudo)
sudo install hydrascale /usr/local/bin/
```

### Using Pre-built Binary

Download the latest release from the releases page and install it:

```bash
sudo install hydrascale /usr/local/bin/
```

## Configuration

Hydrascale uses a YAML configuration file located at `/var/lib/hydrascale/config.yaml` by default.

### Example Configuration

```yaml
tailnets:
  - id: "team-prod"
    exit_node: "node1.example.com"   # optional
  - id: "devops"
    exit_node: null
resolver:
  mode: unified   # only one mode in this release
```

### Configuration Options

- `tailnets`: List of tailnet configurations
  - `id`: Unique identifier for the tailnet (matches your Tailscale tailnet name)
  - `exit_node`: Optional exit node hostname for this tailnet
- `resolver`: DNS resolver configuration
  - `mode`: DNS resolver mode (currently only "unified" is supported)

## Usage

### CLI Commands

Hydrascale provides a command-line interface for managing tailnets:

```bash
# Show help
hydrascale --help

# Add a new tailnet
hydrascale add <tailnet-id>

# Remove a tailnet
hydrascale remove <tailnet-id>

# List all configured tailnets
hydrascale list

# Switch default namespace (for direct tailscale CLI usage)
hydrascale switch <tailnet-id>

# Start Hydrascale in daemon mode
hydrascale serve
```

### Examples

```bash
# Add two tailnets
hydrascale add team-prod
hydrascale add devops

# List configured tailnets
hydrascale list
# Output:
# Listing tailnets:
#   - team-prod
#   - devops

# Remove a tailnet
hydrascale remove devops

# Check status via systemd
sudo systemctl status hydrascale
```

## Running as a Service

Hydrascale is designed to run as a systemd service for production use.

### Installation

1. Copy the binary to `/usr/local/bin/`:
   ```bash
   sudo install hydrascale /usr/local/bin/
   ```

2. Create the systemd user and group:
   ```bash
   sudo useradd --system --no-create-home --shell /usr/false hydrascale
   ```

3. Create the configuration directory:
   ```bash
   sudo mkdir -p /var/lib/hydrascale
   sudo chown hydrascale:hydrascale /var/lib/hydrascale
   ```

4. Install the systemd service file:
   ```bash
   sudo cp dist/hydrascale.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```

5. Configure your tailnets by editing `/var/lib/hydrascale/config.yaml`:
   ```yaml
   tailnets:
     - id: "team-prod"
       exit_node: "node1.example.com"
     - id: "devops"
       exit_node: null
   resolver:
     mode: unified
   ```

6. Enable and start the service:
   ```bash
   sudo systemctl enable hydrascale
   sudo systemctl start hydrascale
   ```

### Service Management

```bash
# Check status
sudo systemctl status hydrascale

# View logs
sudo journalctl -u hydrascale -f

# Stop the service
sudo systemctl stop hydrascale

# Restart the service
sudo systemctl restart hydrascale
```

## How It Works

### Network Namespaces

Each tailnet runs in its own network namespace (`ns-<tailnet-id>`), providing complete network isolation. This ensures that:

- Each tailnet has its own network interfaces, routing tables, and firewall rules
- Traffic from one tailnet cannot interfere with another
- Each tailnet can have overlapping IP ranges without conflicts

### Tailscale Daemon Management

For each tailnet, Hydrascale:

1. Creates a network namespace using `ip netns add`
2. Launches `tailscaled` inside that namespace using `ip netns exec`
3. Manages the daemon lifecycle (start, stop, restart)
4. Synchronizes routes from the daemon to the namespace using netlink

### DNS Resolution

Hydrascale provides a unified DNS resolver that:

- Listens on 127.0.0.1:53 for DNS queries
- Forwards queries to the appropriate tailnet's DNS server based on the domain
- Uses round-robin load balancing when multiple tailnets could handle a domain
- Falls back to public DNS servers (8.8.8.8, 8.8.4.4) for non-tailscale domains

### Route Synchronization

Hydrascale monitors each tailscaled daemon for route changes and:

- Synchronizes routes to the appropriate network namespace using `ip netns exec route add`
- Handles default routes and exit node detection
- Cleans up routes when tailnets are removed

## Security Considerations

### Privileges

Hydrascale requires the following capabilities:
- `CAP_NET_ADMIN`: For network namespace and route management
- `CAP_SYS_PTRACE`: For monitoring daemon processes

When running as a service, it's recommended to:
- Run as a dedicated unprivileged user (`hydrascale`)
- Use ambient capabilities to limit the privilege set
- Consider additional security measures like AppArmor or SELinux profiles

### Isolation

Network namespaces provide strong isolation between tailnets:
- Each tailnet has its own network stack
- No direct communication between namespaces without explicit configuration
- Firewall rules in one namespace don't affect others

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -cover
```

### Building for Development

```bash
# Build with debug information
go build -gcflags="all=-N -l" -o hydrascale ./cmd/hydrascale

# Run the CLI directly
./hydrascale --help
```

## Future Enhancements

See HYPERPLAN.md for planned enhancements including:

- Seccomp/SELinux hardening profiles
- Optional per-namespace DNS stub for enhanced isolation
- Web UI/dashboard for monitoring and management
- Metrics and structured logging for observability
- Improved exit node handling and routing policies

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgements

- Built with [Tailscale](https://tailscale.com)
- Uses [Cobra](https://github.com/spf13/cobra) for CLI functionality
- Uses [miekg/dns](https://github.com/miekg/dns) for DNS resolution