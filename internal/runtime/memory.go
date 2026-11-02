package runtime

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// CheckMemory verifies that this Mac has enough unified memory for the model.
func (s *Setup) CheckMemory(model string) error {
	need, err := ModelMinMemory(model)
	if err != nil {
		return err
	}
	have, err := s.MemoryBytes()
	if err != nil {
		return fmt.Errorf("read memory: %w", err)
	}
	if have < need {
		return fmt.Errorf("%s requires at least %s of unified memory; this Mac has %s", model, formatBytes(need), formatBytes(have))
	}
	return nil
}

func defaultMemoryBytes() (uint64, error) {
	return unix.SysctlUint64("hw.memsize")
}
