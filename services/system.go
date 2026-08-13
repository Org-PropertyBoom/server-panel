package services

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SystemStatus struct {
	CPU     CPUStatus     `json:"cpu"`
	Memory  MemoryStatus  `json:"memory"`
	Storage StorageStatus `json:"storage"`
	Network NetworkStatus `json:"network"`
}

type CPUStatus struct {
	Cores int     `json:"cores"`
	Model string  `json:"model"`
	Usage float64 `json:"usage"`
	// Steal is the share of CPU time the HYPERVISOR took from this VM — time we
	// were runnable but the host ran someone else. It is reported separately and
	// EXCLUDED from Usage: it is not work our processes did, and counting it as
	// "our" load makes a noisy-neighbour host look like a busy server. On a shared
	// cloud instance this is frequently the whole story behind a high reading.
	Steal float64 `json:"steal"`
	// IOWait is time blocked on I/O. Counted as idle (the standard convention, and
	// what top does) but surfaced so disk saturation is distinguishable from
	// genuine idleness.
	IOWait float64 `json:"iowait"`
}

type MemoryStatus struct {
	Total uint64  `json:"total"`
	Used  uint64  `json:"used"`
	Usage float64 `json:"usage"`
}

type StorageStatus struct {
	Total uint64  `json:"total"`
	Used  uint64  `json:"used"`
	Usage float64 `json:"usage"`
}

type NetworkStatus struct {
	Received uint64 `json:"received"`
	Sent     uint64 `json:"sent"`
}

type SystemService struct{}

func NewSystemService() *SystemService { return &SystemService{} }

func (s *SystemService) Status() (SystemStatus, error) {
	cpu, err := cpuStatus()
	if err != nil {
		return SystemStatus{}, err
	}
	memory, err := memoryStatus()
	if err != nil {
		return SystemStatus{}, err
	}
	storage, err := storageStatus("/")
	if err != nil {
		return SystemStatus{}, err
	}
	network, err := networkStatus()
	if err != nil {
		return SystemStatus{}, err
	}

	return SystemStatus{CPU: cpu, Memory: memory, Storage: storage, Network: network}, nil
}

// cpuSample is one reading of the aggregate "cpu" line in /proc/stat.
type cpuSample struct {
	idle   uint64 // idle + iowait
	iowait uint64
	steal  uint64
	total  uint64
	at     time.Time
}

var (
	cpuMu   sync.Mutex
	cpuLast cpuSample // previous reading, so usage is measured across poll intervals
)

// cpuStatus reports CPU utilisation between THIS call and the previous one.
//
// The dashboard polls every few seconds, so differencing against the last sample
// gives a window orders of magnitude wider than sampling inline — and costs no
// wall-clock. The old implementation slept 100ms per request and divided the
// deltas from that. At USER_HZ=100 a 100ms window is ~10 jiffies per core, so on
// a 2-core box a SINGLE busy jiffy moved the reading ~5%: the number was mostly
// quantisation noise, and every caller paid 100ms for it.
//
// The inline sleep is kept only as the cold-start fallback (first call, or after
// a long gap when the cached sample is too stale to difference against).
func cpuStatus() (CPUStatus, error) {
	now, err := cpuTimes()
	if err != nil {
		return CPUStatus{}, err
	}

	cpuMu.Lock()
	prev := cpuLast
	usable := prev.total > 0 && now.total > prev.total && time.Since(prev.at) < 5*time.Minute
	cpuLast = now
	cpuMu.Unlock()

	if !usable {
		// Cold start: no usable previous sample, so take a short one inline.
		time.Sleep(200 * time.Millisecond)
		after, sErr := cpuTimes()
		if sErr != nil {
			return CPUStatus{}, sErr
		}
		cpuMu.Lock()
		cpuLast = after
		cpuMu.Unlock()
		prev, now = now, after
	}

	status := CPUStatus{Cores: runtime.NumCPU(), Model: cpuModel()}
	totalDelta := now.total - prev.total
	if totalDelta == 0 {
		return status, nil
	}

	// These are ALL uint64, so every subtraction here is an underflow risk: a
	// counter that goes backwards wraps to ~1.8e19 instead of going negative, and
	// the dashboard would render a nonsense percentage rather than surface an
	// error. /proc/stat is normally monotonic, but CPU hotplug and counter resets
	// break that. sub() clamps instead of wrapping.
	sub := func(a, b uint64) uint64 {
		if a < b {
			return 0
		}
		return a - b
	}
	pct := func(d uint64) float64 {
		v := float64(d) / float64(totalDelta) * 100
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100 // idle+steal can exceed total only if a counter reset
		}
		return v
	}

	idleDelta, stealDelta := sub(now.idle, prev.idle), sub(now.steal, prev.steal)
	status.Usage = pct(sub(totalDelta, idleDelta+stealDelta))
	status.Steal = pct(stealDelta)
	status.IOWait = pct(sub(now.iowait, prev.iowait))
	return status, nil
}

// cpuTimes reads the aggregate "cpu" line of /proc/stat.
//
// Field order after the "cpu" label is fixed by the kernel:
//
//	0 user  1 nice  2 system  3 idle  4 iowait  5 irq  6 softirq  7 steal  8 guest  9 guest_nice
//
// Two things the previous version got wrong:
//
//   - GUEST AND GUEST_NICE WERE ADDED TO THE TOTAL. The kernel already includes
//     guest inside user, and guest_nice inside nice, so summing every field
//     double-counts them and inflates the denominator. Fields beyond steal are
//     therefore ignored here. (Zero on a non-virtualising host, so this was
//     latent rather than visible — but wrong.)
//   - STEAL WAS COUNTED AS BUSY. It is kept in the total (it is real elapsed
//     time) but returned separately so the caller can exclude it from usage.
func cpuTimes() (cpuSample, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return cpuSample{}, fmt.Errorf("could not read CPU statistics")
	}
	return parseCPULine(scanner.Text())
}

// parseCPULine is the pure half of cpuTimes, split out so the field accounting
// (which steal/guest bugs hid in) is unit-testable without a real /proc/stat.
func parseCPULine(line string) (cpuSample, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSample{}, fmt.Errorf("invalid CPU statistics")
	}

	sample := cpuSample{at: time.Now()}
	// Stop at steal (index 7); guest/guest_nice are already inside user/nice.
	for index, field := range fields[1:] {
		if index > 7 {
			break
		}
		value, parseErr := strconv.ParseUint(field, 10, 64)
		if parseErr != nil {
			return cpuSample{}, parseErr
		}
		sample.total += value
		switch index {
		case 3: // idle
			sample.idle += value
		case 4: // iowait — conventionally counted as idle
			sample.idle += value
			sample.iowait = value
		case 7: // steal
			sample.steal = value
		}
	}
	return sample, nil
}

func cpuModel() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "CPU"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if ok && (strings.TrimSpace(key) == "model name" || strings.TrimSpace(key) == "Hardware") {
			return strings.TrimSpace(value)
		}
	}
	return "CPU"
}

func memoryStatus() (MemoryStatus, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryStatus{}, err
	}
	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 {
		return MemoryStatus{}, fmt.Errorf("memory total is unavailable")
	}
	used := total - available
	return MemoryStatus{Total: total, Used: used, Usage: float64(used) / float64(total) * 100}, nil
}

func storageStatus(path string) (StorageStatus, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return StorageStatus{}, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - available
	usage := 0.0
	if total > 0 {
		usage = float64(used) / float64(total) * 100
	}
	return StorageStatus{Total: total, Used: used, Usage: usage}, nil
}

func networkStatus() (NetworkStatus, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return NetworkStatus{}, err
	}
	defer file.Close()

	var status NetworkStatus
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		name, values, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "lo" {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 9 {
			continue
		}
		received, receiveErr := strconv.ParseUint(fields[0], 10, 64)
		sent, sentErr := strconv.ParseUint(fields[8], 10, 64)
		if receiveErr == nil && sentErr == nil {
			status.Received += received
			status.Sent += sent
		}
	}
	return status, scanner.Err()
}
