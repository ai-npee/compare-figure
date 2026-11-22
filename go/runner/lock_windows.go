//go:build windows

package runner

import "syscall"

func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	handle, err := syscall.OpenProcess(0x00100000, false, uint32(pid))
	if err != nil {
		if err == syscall.Errno(87) {
			return false, nil
		}
		return false, err
	}
	defer syscall.CloseHandle(handle)
	status, err := syscall.WaitForSingleObject(handle, 0)
	return status == 258, err
}
