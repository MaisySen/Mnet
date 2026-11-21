//go:build mnet_enrollment_development

package osownership

import "fmt"

// Absence is conditional OBSERVATION ONLY, never CONFIG detection. An arbitrary
// proc mount/view can hide a leaf even when the kernel consumer is compiled in.
// This error still blocks the profile. It is never converted to an integer0.
type ipv6OperandReadGap struct {
	Path, Config                   string
	ParentOpened, FinalLeafMissing bool
	Cause                          error
}

func (e *ipv6OperandReadGap) Error() string {
	return fmt.Sprintf("unsupported IPv6 operand view %s config=%s parent-open=%t final-leaf-ENOENT=%t: %v", e.Path, e.Config, e.ParentOpened, e.FinalLeafMissing, e.Cause)
}
func (e *ipv6OperandReadGap) Unwrap() error { return e.Cause }
func ipv6OperandPathConfig(path string) string {
	for i, leaf := range ipv6PolicyLeaves {
		if leaf != "" && (path == "ipv6/conf/all/"+leaf || path == "ipv6/conf/default/"+leaf) {
			return ipv6OperandConfig(i)
		}
	}
	return "no reviewed compiled-out group"
}

// Exact emitter guards from ipv6_store_devconf. Compiled-out sets zero in the
// slots due to initial memset, but a zero observation is NOT proof of CONFIG.
func ipv6CompiledOutSlots(config string) []int {
	switch config {
	case "CONFIG_IPV6_ROUTER_PREF":
		return []int{19, 20, 21, 49}
	case "CONFIG_IPV6_ROUTE_INFO":
		return []int{21, 49}
	case "CONFIG_IPV6_OPTIMISTIC_DAD":
		return []int{23, 34}
	case "CONFIG_IPV6_MROUTE":
		return []int{25}
	case "CONFIG_IPV6_SEG6_HMAC":
		return []int{45}
	}
	return nil
}
