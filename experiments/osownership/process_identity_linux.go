//go:build linux && mnet_enrollment_development

package osownership

import (
	"strconv"
	"strings"
)

func readProcessStartTicks(pid int) (uint64, error) {
	b, e := rawReadBounded("/proc/"+strconv.Itoa(pid)+"/stat", 8192)
	if e != nil {
		return 0, e
	}
	return parseProcessStartTicks(string(b), pid)
}
func parseProcessStartTicks(stat string, pid int) (uint64, error) {
	head := strings.IndexByte(stat, ' ')
	tail := strings.LastIndexByte(stat, ')')
	if head < 0 || tail <= head {
		return 0, ErrConflict
	}
	n, e := strconv.Atoi(stat[:head])
	if e != nil || n != pid {
		return 0, ErrConflict
	}
	fields := strings.Fields(stat[tail+1:])
	if len(fields) < 20 {
		return 0, ErrConflict
	}
	ticks, e := strconv.ParseUint(fields[19], 10, 64)
	if e != nil || ticks == 0 {
		return 0, ErrConflict
	}
	return ticks, nil
}
