//go:build linux

package authprivacyhttp

import "golang.org/x/sys/unix"

func feedPerformanceRawClock() (int64, bool) {
	var value unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_MONOTONIC_RAW, &value)
	return value.Nano(), err == nil
}
