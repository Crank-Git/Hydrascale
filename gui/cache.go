package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// diskCache persists the last-known-good daemon snapshot so a client that loses
// its connection shows last-known data (stamped stale) instead of demo fixtures
// or an empty screen. It lives on disk because socketSource is rebuilt on every
// reconnect; the file survives reconnects and app restarts.
//
// Entries are tagged with a connection Key (which daemon they came from). A read
// whose current key differs from the stored one is treated as a miss, and the
// next write resets the file — so pointing the app at a different host never
// surfaces the previous host's data mislabeled as current.
type diskCache struct {
	path string
	mu   sync.Mutex
}

type cachedDetail struct {
	Detail TailnetDetail `json:"detail"`
	At     time.Time     `json:"at"`
}

type cacheFile struct {
	Key       string                  `json:"key"`
	Dashboard *Dashboard              `json:"dashboard,omitempty"`
	DashAt    time.Time               `json:"dashAt"`
	Details   map[string]cachedDetail `json:"details,omitempty"`
}

func cachePath() string {
	return filepath.Join(filepath.Dir(settingsPath()), "cache.json")
}

func newDiskCache(path string) *diskCache { return &diskCache{path: path} }

// read returns the on-disk cache, or an empty file if absent/corrupt.
func (c *diskCache) read() cacheFile {
	var cf cacheFile
	if b, err := os.ReadFile(c.path); err == nil {
		json.Unmarshal(b, &cf)
	}
	return cf
}

// forKey returns the cache contents only if they belong to the given connection
// key; otherwise an empty file (a miss), so a stale entry from a different
// daemon is never used.
func (c *diskCache) forKey(key string) cacheFile {
	c.mu.Lock()
	defer c.mu.Unlock()
	cf := c.read()
	if cf.Key != key {
		return cacheFile{Key: key}
	}
	return cf
}

// putDashboard stores the last-good dashboard under key, resetting the file if
// the key changed (a different daemon).
func (c *diskCache) putDashboard(key string, d Dashboard, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cf := c.read()
	if cf.Key != key {
		cf = cacheFile{Key: key}
	}
	snap := d
	cf.Dashboard = &snap
	cf.DashAt = at
	c.write(cf)
}

// putDetail stores one tailnet's last-good detail under key.
func (c *diskCache) putDetail(key, id string, d TailnetDetail, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cf := c.read()
	if cf.Key != key {
		cf = cacheFile{Key: key}
	}
	if cf.Details == nil {
		cf.Details = map[string]cachedDetail{}
	}
	cf.Details[id] = cachedDetail{Detail: d, At: at}
	c.write(cf)
}

// write persists cf; caller holds the lock. Failures are best-effort (the cache
// is an optimization, not a source of truth).
func (c *diskCache) write(cf cacheFile) {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return
	}
	if b, err := json.MarshalIndent(cf, "", "  "); err == nil {
		os.WriteFile(c.path, b, 0600)
	}
}

// lastSeenLabel renders a cached-at time for the UI ("15:04" today, else date).
func lastSeenLabel(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.Format("Jan 2 15:04")
}
