// wirelesstag-exporter is a Prometheus exporter for WirelessTag.net sensors.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Configuration is loaded from environment variables.
var (
	username string
	password string
)

const (
	listenAddress = ":9189"
	namespace     = "wirelesstag"
)

// Tag represents a single sensor tag from the API response.
type Tag struct {
	Name        string  `json:"name"`
	UUID        string  `json:"uuid"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"cap"`
	SignalDBM   float64 `json:"signaldBm"`
}

// wirelessTagCollector manages all logic for fetching data and creating metrics.
type wirelessTagCollector struct {
	temperatureDesc *prometheus.Desc
	humidityDesc    *prometheus.Desc
	signalDesc      *prometheus.Desc

	client     *http.Client
	loggedIn   bool
	lastLogin  time.Time
	loginMutex sync.Mutex
}

// newWirelessTagCollector initializes the collector.
func newWirelessTagCollector() *wirelessTagCollector {
	labels := []string{"tag_name"}

	// A cookie jar stores and sends cookies automatically, managing the session.
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Failed to create cookie jar: %v", err)
	}

	return &wirelessTagCollector{
		temperatureDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "sensor", "temperature_celsius"),
			"Current temperature in Celsius.",
			labels, nil,
		),
		humidityDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "sensor", "humidity_ratio"),
			"Current relative humidity (0.0 to 1.0).",
			labels, nil,
		),
		signalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "sensor", "signal_dbm"),
			"Signal strength in dBm.",
			labels, nil,
		),
		client: &http.Client{Jar: jar},
	}
}

// Describe implements the prometheus.Collector interface.
func (c *wirelessTagCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.temperatureDesc
	ch <- c.humidityDesc
	ch <- c.signalDesc
}

// Collect implements the prometheus.Collector interface.
func (c *wirelessTagCollector) Collect(ch chan<- prometheus.Metric) {
	c.loginMutex.Lock()
	defer c.loginMutex.Unlock()

	// Re-authenticate every 2 hours to keep the session active.
	if !c.loggedIn || time.Since(c.lastLogin) > 2*time.Hour {
		log.Println("Session expired or not logged in, authenticating...")
		if err := c.authenticate(); err != nil {
			log.Printf("Error: Could not authenticate with API: %v", err)
			return
		}
	}

	tags, err := c.fetchTagData()
	if err != nil {
		log.Printf("Error: Could not fetch data from API: %v", err)
		c.loggedIn = false
		return
	}

	for _, tag := range tags {
		ch <- prometheus.MustNewConstMetric(c.temperatureDesc, prometheus.GaugeValue, tag.Temperature, tag.Name)
		ch <- prometheus.MustNewConstMetric(c.humidityDesc, prometheus.GaugeValue, tag.Humidity/100, tag.Name)
		ch <- prometheus.MustNewConstMetric(c.signalDesc, prometheus.GaugeValue, tag.SignalDBM, tag.Name)
	}
}

// authenticate uses a direct, cookie-based login method, as the documented OAuth2
// flow does not appear to support non-interactive server-side applications.
func (c *wirelessTagCollector) authenticate() error {
	apiURL := "https://www.mytaglist.com/ethAccount.asmx/SignIn"

	payload := map[string]string{
		"email":    username,
		"password": password,
	}
	jsonPayload, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with status: %s", resp.Status)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	log.Println("Successfully authenticated and received session cookie.")
	c.loggedIn = true
	c.lastLogin = time.Now()
	return nil
}

// fetchTagData retrieves the list of tags and their data using the authenticated session.
func (c *wirelessTagCollector) fetchTagData() ([]Tag, error) {
	apiURL := "https://www.mytaglist.com/ethClient.asmx/GetTagList"

	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(`{}`))
	if err != nil {
		return nil, fmt.Errorf("failed to create data request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform data request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("data fetch failed with status: %s", resp.Status)
	}

	body, _ := io.ReadAll(resp.Body)
	var apiResponse struct {
		D []Tag `json:"d"`
	}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse data response JSON: %w", err)
	}

	return apiResponse.D, nil
}

func main() {
	// Load credentials from a .env file in the same directory as the executable.
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on existing environment variables.")
	}

	username = os.Getenv("WTAG_USERNAME")
	password = os.Getenv("WTAG_PASSWORD")

	if username == "" || password == "" {
		log.Fatal("Error: WTAG_USERNAME and WTAG_PASSWORD must be set in the .env file or environment.")
	}

	collector := newWirelessTagCollector()
	prometheus.MustRegister(collector)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
			<html><head><title>WirelessTag Exporter</title></head>
			<body><h1>WirelessTag Exporter</h1><p><a href="/metrics">Metrics</a></p></body>
			</html>
		`))
	})

	log.Printf("Exporter starting. Listening on address %s", listenAddress)
	if err := http.ListenAndServe(listenAddress, nil); err != nil {
		log.Fatalf("Error: Could not start HTTP server: %v", err)
	}
}
