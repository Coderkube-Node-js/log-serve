# log-serve

A lightweight, single-binary web server log monitoring tool written in Go. Stream and analyze Apache, Nginx, and other web server logs in real-time through a beautiful web interface.

## Features

- **Real-time Log Streaming** - Live tail web server logs as they happen
- **Auto Server Detection** - Automatically detects Apache2, Nginx, and other common servers
- **Powerful Filtering** - Filter by level, type, status code, method, IP address, URL, and date range
- **Full-text Search** - Search through logs with instant results
- **Dark Mode** - Toggle between light and dark themes
- **Export Logs** - Export filtered logs for offline analysis
- **Live Statistics** - Real-time counts of total entries, requests, warnings, and errors
- **Responsive Design** - Works on desktop and mobile devices
- **Zero Dependencies** - Single static binary, no external dependencies required

## Supported Servers

- Apache2
- Nginx
- Lighttpd
- Caddy

## Installation

### Quick Install (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/yourusername/log-serve/main/scripts/install.sh | sudo bash
```

### From Source

**Prerequisites:**
- Go 1.20 or later

**Build:**
```bash
git clone https://github.com/yourusername/log-serve.git
cd log-serve
make build
```

**Install:**
```bash
sudo make install
```

### Debian/Ubuntu Package

```bash
sudo dpkg -i log-serve_*.deb
```

## Usage

### Quick Start

```bash
# Run the binary directly
./log-serve

# Or if installed system-wide
log-serve
```

The web interface will be available at `http://localhost:8080`

### Command Line Options

```bash
log-serve [options]

Options:
  -port int       Server port (default 8080)
  -config string  Path to config file
  -log string     Path to log file (auto-detected if not specified)
```

### Systemd Service

When installed via the install script, log-serve runs as a systemd service:

```bash
# Check status
sudo systemctl status log-serve

# Start/Stop/Restart
sudo systemctl start log-serve
sudo systemctl stop log-serve
sudo systemctl restart log-serve

# Enable auto-start on boot
sudo systemctl enable log-serve
```

## Configuration

log-serve automatically detects common log locations. To customize, create a config file:

```yaml
# /etc/log-serve/config.yaml
log_path: /var/log/custom/access.log
server_type: nginx
port: 8080
```

## Building from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/log-serve.git
cd log-serve

# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Clean build artifacts
make clean
```

## Development

```bash
# Run in development mode with auto-reload
make dev

# Run tests
make test

# Generate coverage report
make coverage
```

## Screenshots

![Dashboard](docs/screenshot.png)

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

- [Issues](https://github.com/yourusername/log-serve/issues)
- [Discussions](https://github.com/yourusername/log-serve/discussions)
