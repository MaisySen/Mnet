//go:build linux && mnet_enrollment_development

package osownership

// Only real source verifier can return this private type. The fake setup runner
// does not mint one. Presence alone is not freshness: source users must compare
// resource, observed namespace/name and captured generation before and after.
type originalDeviceEvidence struct {
	resources       *originalHeldPacketResources
	identity        heldPacketIdentity
	deviceNamespace Namespace
	tunName         string
}
