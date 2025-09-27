# WirelessTag Prometheus Exporter

> A simple, standalone Prometheus exporter for WirelessTag.net temperature and humidity sensors.

This exporter logs into your WirelessTag.net account, fetches the latest data from all registered tags, and exposes it in a Prometheus-friendly format.

Currently supports only the Temp/Humidity sensors because that's all what I have.

## Features

* **Standalone**: No external dependencies besides Go to compile.
* **Efficient**: Fetches data only when scraped by Prometheus.
* **Secure**: Configure credentials via a `.env` file, not in the code.
* **Robust**: Designed to be run as a `systemd` service.

## Installation

You must have a recent version of Go installed (I compiled with 1.24.7).

```bash
# Clone the repository
git clone [https://github.com/styygeli/wirelesstag-exporter.git](https://github.com/styygeli/wirelesstag-exporter.git)
cd wirelesstag-exporter

# Tidy dependencies
go mod tidy

# Build the optimized binary
go build -ldflags="-s -w"
```
This will create a `wirelesstag-exporter` executable in the directory.

## Configuration

The exporter is configured via a file named `.env` placed in the same directory as the executable.

1.  Create a file named `.env`:
    ```bash
    nano .env
    ```

2.  Add your credentials:
    ```ini
    WTAG_USERNAME="your-email@example.com"
    WTAG_PASSWORD="your-wirelesstag-password"
    ```

## Running the Exporter

### For Testing

You can run the exporter directly from your terminal.

```bash
./wirelesstag-exporter
```
The exporter will start on port `9189`. You can now test the metrics endpoint:
```bash
curl http://localhost:9189/metrics
```

### As a `systemd` Service

1.  Move the compiled binary and the `.env` file to a dedicated directory:
    ```bash
    mkdir -p /home/your-user/wirelesstag-exporter
    mv wirelesstag-exporter .env /home/your-user/wirelesstag-exporter/
    ```

2.  Create a `systemd` service file at `/etc/systemd/system/wirelesstag-exporter.service`:
    ```ini
    [Unit]
    Description=Prometheus Exporter for WirelessTag.net
    Wants=network-online.target
    After=network-online.target

    [Service]
    User=your-user
    Group=your-user
    
    WorkingDirectory=/home/your-user/wirelesstag-exporter
    ExecStart=/home/your-user/wirelesstag-exporter/wirelesstag-exporter
    
    Restart=on-failure
    RestartSec=5s

    [Install]
    WantedBy=multi-user.target
    ```

3.  Enable and start the service:
    ```bash
    sudo systemctl daemon-reload
    sudo systemctl enable --now wirelesstag-exporter.service
    sudo systemctl status wirelesstag-exporter.service
    ```

## Exposed Metrics

The exporter exposes the following metrics:

| Metric                                      | Labels     | Description                                |
| ------------------------------------------- | ---------- | ------------------------------------------ |
| `wirelesstag_sensor_temperature_celsius`    | `tag_name` | Current temperature in Celsius.            |
| `wirelesstag_sensor_humidity_ratio`         | `tag_name` | Current relative humidity (0.0 to 1.0).    |
| `wirelesstag_sensor_signal_dbm`             | `tag_name` | Signal strength in dBm.                    |

It also includes standard Go process and `promhttp` metrics for monitoring the exporter's own health.

## License

This project is licensed under the MIT License.
