//go:build !linux && mnet_enrollment_development

package daemon

func newDevelopmentEndpointNotifier() (developmentEndpointNotifier, error) {
	return nil, errPrivateEndpoint
}
