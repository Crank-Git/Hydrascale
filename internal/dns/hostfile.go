package dns

import (
	"sync"
	"time"
)

// DefaultHostResolvConf is the host resolv.conf path that the daemon observes.
const DefaultHostResolvConf = "/etc/resolv.conf"

// HostFileState holds one observation of the host resolv.conf file.
type HostFileState struct {
	// Path is the observed path.
	Path string
	// LinkTarget is the target of the symbolic link, or an empty string when the path
	// is a regular file.
	LinkTarget string
	// Checksum is the SHA-256 checksum, or an empty string when the file is missing.
	Checksum string
	// FirstLine is the first line of the file, or an empty string when the file is
	// missing. The state holds no other line, because a resolv.conf file can contain a
	// search domain that names an internal host.
	FirstLine string
	// Missing is true when the file does not exist.
	Missing bool
}

// ReadHostFileState returns the observed state of the file at path.
// The checksum covers the link target path and the content of the target, so a change to
// the symbolic link also changes the checksum. A missing file gives an empty checksum and
// Missing true. ReadHostFileState returns an error when it cannot read a file that exists.
func ReadHostFileState(path string) (HostFileState, error) {
	// TODO(#77): Implement the checksum reader.
	return HostFileState{Path: path}, nil
}

// HostFileMonitor reports a change to the host resolv.conf file.
// The monitor reports the change. The monitor does not write the host file.
type HostFileMonitor struct {
	mu         sync.Mutex
	path       string
	started    bool
	state      HostFileState
	lastChange time.Time
}

// NewHostFileMonitor returns a monitor for the file at path.
// An empty path selects DefaultHostResolvConf.
func NewHostFileMonitor(path string) *HostFileMonitor {
	if path == "" {
		path = DefaultHostResolvConf
	}
	return &HostFileMonitor{path: path}
}

// Start records the first checksum of the file and returns the observed state.
// Start returns an error when it cannot read a file that exists.
func (m *HostFileMonitor) Start() (HostFileState, error) {
	// TODO(#77): Implement the start observation.
	return HostFileState{Path: m.path}, nil
}

// Check compares the current checksum with the recorded checksum.
// Check returns the previous state and the current state. Check stores the current state
// as the recorded state before it returns, so one change gives changed true one time. The
// first call records the state and returns changed false. Check returns an error when it
// cannot read a file that exists, and it keeps the recorded state on an error.
func (m *HostFileMonitor) Check() (changed bool, previous, current HostFileState, err error) {
	// TODO(#77): Implement the checksum comparison.
	return false, HostFileState{Path: m.path}, HostFileState{Path: m.path}, nil
}

// State returns the recorded state and the time of the last change.
// The time is the zero time when the monitor reports no change.
func (m *HostFileMonitor) State() (HostFileState, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.lastChange
}
