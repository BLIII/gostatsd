package statsd

import (
	"strconv"
	"time"

	"github.com/jtblin/gostatsd/backend"
	_ "github.com/jtblin/gostatsd/backend/backends"
	"github.com/jtblin/gostatsd/types"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// StatsdServer encapsulates all of the parameters necessary for starting up
// the statsd server. These can either be set via command line or directly.
type StatsdServer struct {
	Backends         []string
	ConfigPath       string
	ConsoleAddr      string
	ExpiryInterval   time.Duration
	FlushInterval    time.Duration
	MetricsAddr      string
	Namespace        string
	PercentThreshold []string
	Verbose          bool
	Version          bool
	WebConsoleAddr   string
}

// NewStatsdServer will create a new StatsdServer with default values.
func NewStatsdServer() *StatsdServer {
	return &StatsdServer{
		Backends:         []string{"graphite"},
		MetricsAddr:      ":8125",
		ConsoleAddr:      ":8126",
		WebConsoleAddr:   ":8181",
		ExpiryInterval:   5 * time.Minute,
		FlushInterval:    1 * time.Second,
		PercentThreshold: []string{"90"},
	}
}

// AddFlags adds flags for a specific DockerAuthServer to the specified FlagSet
func (s *StatsdServer) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&s.Backends, "backends", s.Backends, "Comma-separated list of backends")
	fs.StringVar(&s.ConfigPath, "config-path", s.ConfigPath, "Path to the configuration file")
	fs.DurationVar(&s.ExpiryInterval, "expiry-interval", s.ExpiryInterval, "After how long do we expire metrics (0 to disable)")
	fs.DurationVar(&s.FlushInterval, "flush-interval", s.FlushInterval, "How often to flush metrics to the backends")
	fs.StringVar(&s.MetricsAddr, "metrics-addr", s.MetricsAddr, "Address on which to listen for metrics")
	fs.StringVar(&s.Namespace, "namespace", s.Namespace, "Namespace all metrics")
	fs.StringVar(&s.WebConsoleAddr, "web-addr", s.WebConsoleAddr, "If set, use as the address of the web-based console")
	fs.StringVar(&s.ConsoleAddr, "console-addr", s.ConsoleAddr, "If set, use as the address of the telnet-based console")
	fs.StringSliceVar(&s.PercentThreshold, "percent-threshold", s.PercentThreshold, "Comma-separated list of percentiles")
	fs.BoolVar(&s.Verbose, "verbose", false, "Verbose")
	fs.BoolVar(&s.Version, "version", false, "Print the version and exit")
}

// Run runs the specified StatsdServer.
func (s *StatsdServer) Run() error {
	if s.Verbose {
		log.SetLevel(log.DebugLevel)
	}

	if s.ConfigPath != "" {
		viper.SetConfigFile(s.ConfigPath)
		err := viper.ReadInConfig()
		if err != nil {
			return err
		}
	}

	// Start the metric aggregator
	var backends []backend.MetricSender
	for _, backendName := range s.Backends {
		b, err := backend.InitBackend(backendName)
		if err != nil {
			return err
		}
		backends = append(backends, b)
	}

	var percentThresholds []float64
	for _, sPercentThreshold := range s.PercentThreshold {
		pt, err := strconv.ParseFloat(sPercentThreshold, 64)
		if err != nil {
			return err
		}
		percentThresholds = append(percentThresholds, pt)
	}

	aggregator := NewMetricAggregator(backends, percentThresholds, s.FlushInterval, s.ExpiryInterval)
	go aggregator.Aggregate()

	// Start the metric receiver
	f := func(metric types.Metric) {
		aggregator.MetricChan <- metric
	}
	receiver := MetricReceiver{s.MetricsAddr, s.Namespace, HandlerFunc(f)}
	go receiver.ListenAndReceive()

	// Start the console(s)
	if s.ConsoleAddr != "" {
		console := ConsoleServer{s.ConsoleAddr, &aggregator}
		go console.ListenAndServe()
	}
	if s.WebConsoleAddr != "" {
		console := WebConsoleServer{s.WebConsoleAddr, &aggregator}
		go console.ListenAndServe()
	}

	// Listen forever
	select {}

	return nil
}