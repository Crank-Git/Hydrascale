package dns

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
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
	state := HostFileState{Path: path}

	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		state.Missing = true
		return state, nil
	case err != nil:
		return state, fmt.Errorf("stat %s: %w", path, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return state, fmt.Errorf("read link %s: %w", path, err)
		}
		state.LinkTarget = target
	}

	content, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// The symbolic link exists and the target does not exist.
		state.Missing = true
		return state, nil
	case err != nil:
		return state, fmt.Errorf("read %s: %w", path, err)
	}

	sum := sha256.New()
	// The link target path is part of the checksum, so a change to the symbolic link
	// alone also reports as a change.
	if state.LinkTarget != "" {
		fmt.Fprintf(sum, "link:%s\n", state.LinkTarget)
	}
	sum.Write(content)
	state.Checksum = hex.EncodeToString(sum.Sum(nil))
	state.FirstLine = firstLine(content)
	return state, nil
}

// firstLine returns the first line of content, without the line terminator.
func firstLine(content []byte) string {
	line := string(content)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSuffix(line, "\r")
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
	current, err := ReadHostFileState(m.path)
	if err != nil {
		return current, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	m.state = current
	return current, nil
}

// Check compares the current checksum with the recorded checksum.
// Check returns the previous state and the current state. Check stores the current state
// as the recorded state before it returns, so one change gives changed true one time. The
// first call records the state and returns changed false. Check returns an error when it
// cannot read a file that exists, and it keeps the recorded state on an error.
func (m *HostFileMonitor) Check() (changed bool, previous, current HostFileState, err error) {
	current, err = ReadHostFileState(m.path)

	m.mu.Lock()
	defer m.mu.Unlock()
	previous = m.state
	if err != nil {
		return false, previous, current, err
	}

	if !m.started {
		m.started = true
		m.state = current
		return false, previous, current, nil
	}

	if previous.Checksum == current.Checksum &&
		previous.LinkTarget == current.LinkTarget &&
		previous.Missing == current.Missing {
		return false, previous, current, nil
	}

	// The monitor stores the current state now, so one change gives one report.
	m.state = current
	m.lastChange = time.Now()
	return true, previous, current, nil
}

// State returns the recorded state and the time of the last change.
// The time is the zero time when the monitor reports no change.
func (m *HostFileMonitor) State() (HostFileState, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.lastChange
}
