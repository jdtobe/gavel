package runtime

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// CheckDisk verifies that the target filesystem has at least required bytes.
func (s *Setup) CheckDisk(path string, required uint64) error {
	free, err := s.DiskFree(path)
	if err != nil {
		return fmt.Errorf("disk free: %w", err)
	}
	if free < required {
		return fmt.Errorf("not enough disk space: required %s, free %s", formatBytes(required), formatBytes(free))
	}
	return nil
}

func defaultDiskFree(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

func formatBytes(n uint64) string {
	const (
		gib = 1 << 30
		mib = 1 << 20
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/gib)
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/mib)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
