package runner

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type runLock struct {
	path string
	data []byte
}
type lockOwner struct {
	PID     int    `json:"pid"`
	Host    string `json:"host"`
	Nonce   string `json:"nonce"`
	Started string `json:"started"`
}

func acquire(root string) (*runLock, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, ".runner.lock")
	if filepath.Dir(path) != root {
		return nil, fmt.Errorf("invalid lock target")
	}
	host, _ := os.Hostname()
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return nil, err
	}
	record := lockOwner{os.Getpid(), host, hex.EncodeToString(token), time.Now().UTC().Format(time.RFC3339Nano)}
	data, _ := json.Marshal(record)
	for attempts := 0; attempts < 2; attempts++ {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e == nil {
			_, e = f.Write(data)
			_ = f.Sync()
			closeErr := f.Close()
			if e != nil {
				return nil, e
			}
			if closeErr != nil {
				return nil, closeErr
			}
			return &runLock{path, data}, nil
		}
		if !os.IsExist(e) {
			return nil, e
		}
		before, e := os.ReadFile(path)
		if e != nil {
			continue
		}
		var owner lockOwner
		if e = json.Unmarshal(before, &owner); e != nil {
			return nil, fmt.Errorf("incomplete workspace lock; retry after the current writer finishes")
		}
		if owner.Host != host {
			return nil, fmt.Errorf("workspace locked by host %s pid %d", owner.Host, owner.PID)
		}
		alive, e := processAlive(owner.PID)
		if e != nil || alive {
			return nil, fmt.Errorf("workspace already running (pid %d)", owner.PID)
		}
		current, e := os.ReadFile(path)
		if e != nil {
			continue
		}
		if !bytes.Equal(current, before) {
			continue
		}
		if filepath.IsAbs(path) && filepath.Base(path) == ".runner.lock" {
			if e = os.Remove(path); e != nil && !os.IsNotExist(e) {
				return nil, e
			}
		}
	}
	return nil, fmt.Errorf("workspace lock changed; retry")
}
func (lock *runLock) release() {
	if lock == nil {
		return
	}
	current, e := os.ReadFile(lock.path)
	if e == nil && bytes.Equal(current, lock.data) && filepath.IsAbs(lock.path) && filepath.Base(lock.path) == ".runner.lock" {
		_ = os.Remove(lock.path)
	}
}
