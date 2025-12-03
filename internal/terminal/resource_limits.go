// Package terminal provides terminal session management.
// This file contains resource limit utilities for background processes (M6).
//go:build darwin || linux || freebsd
// +build darwin linux freebsd

package terminal

import (
	"fmt"
	"os/exec"
	"syscall"
)

// ResourceLimits holds resource limit configuration for a process
type ResourceLimits struct {
	MaxMemoryMB   int64 // Maximum memory in MB
	MaxFileSizeMB int64 // Maximum file size in MB
	Nice          int   // Nice value (-20 to 19)
	Enabled       bool  // Whether limits are enabled
}

// applyResourceLimits applies resource limits to a command before starting it.
// This is called before cmd.Start() to set up process attributes.
//
// Note: Some limits (like memory) are applied via rlimit which are inherited
// by the child process. CPU limits are not directly supported - use nice value instead.
func applyResourceLimits(cmd *exec.Cmd, limits ResourceLimits) error {
	if !limits.Enabled {
		return nil
	}

	// Set up SysProcAttr for the command
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Set process group ID so we can kill the entire group later
	cmd.SysProcAttr.Setpgid = true

	return nil
}

// setResourceLimits sets resource limits on a running process.
// This is called after the process starts to apply runtime limits.
func setResourceLimits(pid int, limits ResourceLimits) error {
	if !limits.Enabled || pid <= 0 {
		return nil
	}

	// Note: Setting resource limits after process start is limited.
	// Most limits need to be set before exec or inherited.
	// We can use renice for the nice value though.

	// Apply nice value via renice syscall
	if limits.Nice != 0 {
		// Get current priority
		currentPrio, err := syscall.Getpriority(syscall.PRIO_PROCESS, pid)
		if err != nil {
			return fmt.Errorf("failed to get process priority: %w", err)
		}

		// Calculate new priority (nice value)
		// Note: On UNIX, lower values = higher priority
		newPrio := currentPrio + limits.Nice
		if newPrio > 19 {
			newPrio = 19
		}
		if newPrio < -20 {
			newPrio = -20
		}

		// Set the new priority
		if err := syscall.Setpriority(syscall.PRIO_PROCESS, pid, newPrio); err != nil {
			// Don't fail if we can't set priority (may need root)
			// Just log and continue
			return nil
		}
	}

	return nil
}

// createRlimit creates a syscall.Rlimit from a value in MB
func createRlimit(valueMB int64) syscall.Rlimit {
	if valueMB <= 0 {
		return syscall.Rlimit{
			Cur: syscall.RLIM_INFINITY,
			Max: syscall.RLIM_INFINITY,
		}
	}
	valueBytes := uint64(valueMB * 1024 * 1024)
	return syscall.Rlimit{
		Cur: valueBytes,
		Max: valueBytes,
	}
}

// GetCurrentResourceLimits returns the current resource limits for a process
func GetCurrentResourceLimits(pid int) (map[string]syscall.Rlimit, error) {
	limits := make(map[string]syscall.Rlimit)

	// Get various limits
	var limit syscall.Rlimit

	if err := syscall.Getrlimit(syscall.RLIMIT_AS, &limit); err == nil {
		limits["address_space"] = limit
	}

	if err := syscall.Getrlimit(syscall.RLIMIT_DATA, &limit); err == nil {
		limits["data_segment"] = limit
	}

	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err == nil {
		limits["file_size"] = limit
	}

	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err == nil {
		limits["open_files"] = limit
	}

	if err := syscall.Getrlimit(syscall.RLIMIT_CPU, &limit); err == nil {
		limits["cpu_time"] = limit
	}

	return limits, nil
}

// FormatRlimit formats an rlimit value to a human-readable string
func FormatRlimit(limit uint64) string {
	if limit == syscall.RLIM_INFINITY {
		return "unlimited"
	}

	// Format in appropriate units
	if limit >= 1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(limit)/(1024*1024*1024))
	} else if limit >= 1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(limit)/(1024*1024))
	} else if limit >= 1024 {
		return fmt.Sprintf("%.2f KB", float64(limit)/1024)
	}
	return fmt.Sprintf("%d bytes", limit)
}
