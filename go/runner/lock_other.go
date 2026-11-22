//go:build !unix && !windows

package runner

import "fmt"

func processAlive(pid int) (bool, error) {
	return true, fmt.Errorf("process liveness unavailable on this platform")
}
