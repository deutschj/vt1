package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type State struct {
	Timestamp       time.Time `json:"timestamp"`
	TempC           float64   `json:"temp_c"`
	VoltV           float64   `json:"volt_v"`
	ClockArmMHz     float64   `json:"clock_arm_mhz"`
	ThrottleHex     string    `json:"throttle_hex"`
	Undervoltage    bool      `json:"undervoltage"`
	FreqCapped      bool      `json:"freq_capped"`
	Throttled       bool      `json:"throttled"`
	Source          string    `json:"source"`
	LastPollLatency string    `json:"last_poll_latency"`

	// Debug helpers
	RawTemp     string `json:"raw_temp,omitempty"`
	RawVolts    string `json:"raw_volts,omitempty"`
	RawThrottle string `json:"raw_throttle,omitempty"`
	RawClock    string `json:"raw_clock,omitempty"`

	// Error visibility
	LastError   string    `json:"last_error,omitempty"`
	LastErrorAt time.Time `json:"last_error_at,omitempty"`
}

type cache struct {
	mu    sync.RWMutex
	state State
}

func (c *cache) Get() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}
func (c *cache) Set(s State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = s
}

var debug bool

func dbg(format string, args ...any) {
	if debug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	dbg("exec: %s %s", name, strings.Join(args, " "))
	out, err := cmd.CombinedOutput()
	sout := strings.TrimSpace(string(out))
	if err != nil {
		// include combined output in the error
		wrapped := fmt.Errorf("exec failed: %s %s: %w; output: %q",
			name, strings.Join(args, " "), err, sout)
		log.Println(wrapped)
		return sout, wrapped
	}
	dbg("exec ok: %s %s -> %q", name, strings.Join(args, " "), sout)
	return sout, nil
}

func parseTemp(out string) (float64, error) {
	// expected "temp=53.2'C"
	o := out
	o = strings.TrimPrefix(o, "temp=")
	o = strings.TrimSuffix(o, "'C")
	v, err := strconv.ParseFloat(o, 64)
	if err != nil {
		return 0, fmt.Errorf("parseTemp: out=%q stripped=%q: %w", out, o, err)
	}
	return v, nil
}

func parseVolts(out string) (float64, error) {
	// expected "volt=0.8625V"
	o := out
	o = strings.TrimPrefix(o, "volt=")
	o = strings.TrimSuffix(o, "V")
	v, err := strconv.ParseFloat(o, 64)
	if err != nil {
		return 0, fmt.Errorf("parseVolts: out=%q stripped=%q: %w", out, o, err)
	}
	return v, nil
}

func parseClock(out string) (float64, error) {
	// expected "frequency(48)=1500398464"
	parts := strings.Split(out, "=")
	if len(parts) != 2 {
		return 0, fmt.Errorf("parseClock: unexpected format %q", out)
	}
	hz, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parseClock: %q: %w", parts[1], err)
	}
	mhz := hz / 1e6
	return math.Round(mhz*10) / 10, nil
}

func parseThrottleBits(out string) (hex string, uv, fc, thr bool, err error) {
	// e.g. "throttled=0x0" or "throttled=0x50005"
	hex = out
	if strings.HasPrefix(out, "throttled=") {
		hex = strings.TrimPrefix(out, "throttled=")
	}
	val, e := strconv.ParseUint(strings.TrimPrefix(hex, "0x"), 16, 64)
	if e != nil {
		return hex, false, false, false, fmt.Errorf("parseThrottleBits: %q: %w", out, e)
	}
	// current-state bits in low 16
	uv = (val & (1 << 0)) != 0
	fc = (val & (1 << 1)) != 0
	thr = (val & (1 << 2)) != 0
	return hex, uv, fc, thr, nil
}

func pollOnce(timeout time.Duration) (State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()

	tOut, tErr := run(ctx, "vcgencmd", "measure_temp")
	vOut, vErr := run(ctx, "vcgencmd", "measure_volts")
	thOut, thErr := run(ctx, "vcgencmd", "get_throttled")
	clkOut, cErr := run(ctx, "vcgencmd", "measure_clock", "arm")

	var firstErr error
	if tErr != nil && firstErr == nil {
		firstErr = tErr
	}
	if vErr != nil && firstErr == nil {
		firstErr = vErr
	}
	if thErr != nil && firstErr == nil {
		firstErr = thErr
	}
	if cErr != nil && firstErr == nil {
		firstErr = cErr
	}
	if firstErr != nil {
		return State{
			Timestamp:       time.Now(),
			Source:          "vcgencmd",
			RawTemp:         tOut,
			RawVolts:        vOut,
			RawThrottle:     thOut,
			RawClock:        clkOut,
			LastPollLatency: time.Since(start).String(),
			LastError:       firstErr.Error(),
			LastErrorAt:     time.Now(),
		}, firstErr
	}

	temp, err := parseTemp(tOut)
	if err != nil {
		return State{RawTemp: tOut, LastError: err.Error(), LastErrorAt: time.Now()}, err
	}
	volt, err := parseVolts(vOut)
	if err != nil {
		return State{RawVolts: vOut, LastError: err.Error(), LastErrorAt: time.Now()}, err
	}
	clockMHz, err := parseClock(clkOut)
	if err != nil {
		return State{RawClock: clkOut, LastError: err.Error(), LastErrorAt: time.Now()}, err
	}
	thHex, uv, fc, thr, err := parseThrottleBits(thOut)
	if err != nil {
		return State{RawThrottle: thOut, LastError: err.Error(), LastErrorAt: time.Now()}, err
	}

	s := State{
		Timestamp:       time.Now(),
		TempC:           temp,
		VoltV:           volt,
		ClockArmMHz:     clockMHz,
		ThrottleHex:     thHex,
		Undervoltage:    uv,
		FreqCapped:      fc,
		Throttled:       thr,
		Source:          "vcgencmd",
		LastPollLatency: time.Since(start).String(),

		RawTemp:     tOut,
		RawVolts:    vOut,
		RawThrottle: thOut,
		RawClock:    clkOut,
	}
	return s, nil
}

func main() {
	listen := flag.String("listen", ":8085", "HTTP listen address")
	poll := flag.Duration("poll-interval", 5*time.Second, "vcgencmd poll interval")
	timeout := flag.Duration("poll-timeout", 800*time.Millisecond, "timeout per vcgencmd")
	flag.BoolVar(&debug, "debug", false, "enable verbose debug logging")
	flag.Parse()

	// Allow env DEBUG=1 as well
	if !debug && os.Getenv("LOG_LEVEL") == "DEBUG" {
		debug = true
	}
	if debug {
		log.Printf("[DEBUG] debug logging enabled")
	}

	// Helpful preflight: ensure vcgencmd exists
	if _, err := exec.LookPath("vcgencmd"); err != nil {
		log.Printf("WARN: vcgencmd not found in PATH: %v", err)
		log.Printf("      Typically available on Raspberry Pi OS. If running in a container, you may need to install it on the host and mount it, or run agent on host.")
	}

	var c cache

	// Initial poll (non-fatal)
	if s, err := pollOnce(*timeout); err != nil {
		log.Printf("initial poll failed: %v", err)
		c.Set(s) // still set state so /power shows last_error
	} else {
		c.Set(s)
	}

	// Background poller
	go func() {
		t := time.NewTicker(*poll)
		defer t.Stop()
		for range t.C {
			s, err := pollOnce(*timeout)
			if err != nil {
				log.Printf("poll error: %v", err)
			}
			c.Set(s)
			dbg("polled: temp=%.2fC volt=%.3fV arm=%.1fMHz uv=%v thr=%v fc=%v",
				s.TempC, s.VoltV, s.ClockArmMHz, s.Undervoltage, s.Throttled, s.FreqCapped)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/power", func(w http.ResponseWriter, r *http.Request) {
		st := c.Get()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(st); err != nil {
			log.Printf("write /power error: %v", err)
		}
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})

	log.Printf("power-agent listening on %s (poll=%s, timeout=%s)", *listen, poll.String(), timeout.String())
	log.Fatal(http.ListenAndServe(*listen, mux))
}

