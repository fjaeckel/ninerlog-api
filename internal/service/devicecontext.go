package service

import "context"

type deviceContextKey struct{}

// DeviceInfo describes the client a session is being created or renewed from.
type DeviceInfo struct {
	UserAgent string
	IPAddress string
}

// ContextWithDevice returns ctx carrying the calling client's details.
func ContextWithDevice(ctx context.Context, info DeviceInfo) context.Context {
	return context.WithValue(ctx, deviceContextKey{}, info)
}

// DeviceFromContext reads the calling client's details. Returns the zero value
// when nothing was attached.
func DeviceFromContext(ctx context.Context) DeviceInfo {
	info, _ := ctx.Value(deviceContextKey{}).(DeviceInfo)
	return info
}
