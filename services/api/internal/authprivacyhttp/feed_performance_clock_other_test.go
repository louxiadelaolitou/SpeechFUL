//go:build !linux

package authprivacyhttp

func feedPerformanceRawClock() (int64, bool) { return 0, false }
