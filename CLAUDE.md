# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**k3air** is a CLI tool written in Go that enables automated installation of high-availability k3s clusters in offline (air-gap) environments. All components run locally without external dependencies.

### Key Features
- Offline k3s cluster installation
- YAML-based configuration for cluster definition
- Multi-server control plane with HA support
- SSH-based deployment (password and key authentication)
- Container image bundles for air-gap deployments
- Automatic systemd service configuration

## Build and Run

```bash
# Build
go build -o k3air

# Generate default configuration
k3air init

# Deploy cluster
k3air apply -f init.yaml
```

## Architecture

```
k3air/
├── main.go                      # CLI entry point (init, apply commands)
├── internal/
│   ├── config/config.go         # YAML config structs and loading
│   ├── install/install.go       # Core installation logic
│   └── sshclient/sshclient.go   # SSH/SFTP wrapper with progress bars
├── assets/                      # Binary artifacts (not in git)
│   ├── k3s                      # K3s binary
│   ├── k3s-airgap-images-amd64.tar.gz  # Container images
│   └── k3s-uninstall.sh         # Cleanup script
└── init.yaml                    # Default configuration template
```

## Configuration Structure

Configuration is defined in YAML with three main sections:

```yaml
cluster:          # Network and cluster settings
    flannel-backend: "vxlan"
    cluster-cidr: "10.42.0.0/16"
    service-cidr: "10.43.0.0/16"
    token: "cluster-token"
    tls-san: ["192.168.1.10", "k3s.example.com"]
    disable: ["traefik", "metrics-server"]
    data-dir: "/var/lib/rancher/k3s"
    registries: |              # Private registry configuration (optional)

servers:          # Control plane nodes (first server is primary)
    - node_name: "server-1"
      ip: "192.168.1.10"
      port: 22
      user: "root"
      password: "password"     # Or use key_path instead
      key_path: "/root/.ssh/id_rsa"
      labels: ["disk=ssd"]

agents:           # Worker nodes
    - node_name: "agent-1"
      ip: "192.168.1.20"
      ...
```

### Default Values
If not specified in config, these defaults apply:
- `cluster-cidr`: `10.42.0.0/16`
- `service-cidr`: `10.43.0.0/16`
- `data-dir`: `/var/lib/rancher/k3s`
- `flannel-backend`: `vxlan`
- `user`: `root`

## Implementation Details

### Primary Server Election
- The **first server** in the `servers` list becomes the primary
- Primary server starts with `--cluster-init` flag
- Subsequent servers join via `--server https://primaryIP:6443`
- All agents also join the primary server's API endpoint

### Installation Flow (internal/install/install.go)
1. **prepareNode**: Create directories (`/usr/local/bin`, images dir, `/etc/rancher/k3s`)
2. **uploadAssets**: Deploy k3s binary, airgap images, registries.yaml
3. **Create systemd service**: Generate unit file with k3s args
4. **Enable and start**: `systemctl enable/restart k3s` or `k3s-agent`
5. **Post-install**: Copy k3s binary to kubectl for convenience

### SSH Client (internal/sshclient/sshclient.go)
- Supports both password and key-based authentication
- SFTP for file uploads with optional progress bars
- Uses `ssh.InsecureIgnoreHostKey()` (known environment)
- 20-second connection timeout

## Dependencies

- Go 1.22
- `gopkg.in/yaml.v3` - YAML parsing
- `github.com/pkg/sftp` - SFTP client
- `golang.org/x/crypto` - SSH/crypto libraries
- `github.com/schollz/progressbar/v3` - Upload progress bars

## Deployment Requirements

1. Linux kernel > 3.10 on all nodes
2. NTP time synchronization across nodes
3. Port 6443 available on server nodes
4. SSH access (password or key) to all nodes

## Code Conventions

- Standard Go project layout with `internal/` packages
- Config structs use YAML tags with snake_case
- Errors are wrapped with context (see `runCmd` in install.go)
- Uses `log/slog` for structured logging
