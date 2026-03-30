package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type State struct {
	Version int    `json:"version"`
	PeerAPI string `json:"peerAPI,omitempty"`
}

func StartDaemon(tailnet string, namespaceName string, args ...string) error {
	stateFile := "/tmp/tailscaled.state"
	args = append(args, "--state-dir=/tmp/tailscaled", "--tailnet="+tailnet)

	if len(namespaceName) > 0 {
		cmd := exec.Command("ip", "netns", "exec", namespaceName, "tailscaled")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to start tailscaled in namespace %q: %s (%v)", namespaceName, output, err)
		}
	} else {
		cmd := exec.Command("tailscaled", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to start tailscaled: %s (%v)", string(output), err)
		}
	}

	state, err := readState(stateFile)
	if err == nil && state.Version > 0 {
		fmt.Printf("tailscaled running with peerAPI: %s\n", state.PeerAPI)
	} else {
		fmt.Printf("tailscaled started in namespace %q\n", namespaceName)
	}

	return nil
}

func StopDaemon() error {
	cmd := exec.Command("pkill", "-9", "tailscaled")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop tailscaled: %s (%v)", string(output), err)
	}
	fmt.Printf("tailscaled stopped\n")
	return nil
}

func readState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func PollState(pollInterval time.Duration) (<-chan *State, error) {
	stateChan := make(chan *State, 1)
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for range ticker.C {
			state, err := readState("/tmp/tailscaled.state")
			if err == nil && state.Version > 0 {
				select {
				case stateChan <- state:
				default:
				}
			}
		}
	}()
	return stateChan, nil
}
