package service

import "strings"

// maxUserAgentLength bounds the stored User-Agent to the column width.
const maxUserAgentLength = 512

// UnknownDeviceLabel is the label for a User-Agent nothing was recognised in.
const UnknownDeviceLabel = "Unknown device"

// DeviceLabel renders a User-Agent as a short "<browser> on <platform>" label
// for the session list. Returns "Unknown device" when nothing is recognised.
func DeviceLabel(userAgent string) string {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return UnknownDeviceLabel
	}

	client := detectClient(ua)
	platform := detectPlatform(ua)

	switch {
	case client != "" && platform != "":
		return client + " on " + platform
	case client != "":
		return client
	case platform != "":
		return platform
	default:
		return UnknownDeviceLabel
	}
}

// detectClient names the browser or app behind a User-Agent, or "" when it
// matches none of the known ones.
func detectClient(ua string) string {
	switch {
	case strings.Contains(ua, "NinerLog"):
		return "NinerLog app"
	case strings.Contains(ua, "Edg/"):
		return "Edge"
	case strings.Contains(ua, "OPR/"), strings.Contains(ua, "Opera"):
		return "Opera"
	case strings.Contains(ua, "SamsungBrowser"):
		return "Samsung Internet"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS"):
		return "Firefox"
	case strings.Contains(ua, "CriOS"):
		return "Chrome"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "Chromium"):
		return "Chrome"
	case strings.Contains(ua, "Safari/"):
		return "Safari"
	case strings.Contains(ua, "curl/"):
		return "curl"
	default:
		return ""
	}
}

func detectPlatform(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "iOS"):
		return "iOS"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		return "macOS"
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	default:
		return ""
	}
}

// truncateUserAgent bounds a User-Agent to maxUserAgentLength.
func truncateUserAgent(ua string) string {
	if len(ua) <= maxUserAgentLength {
		return ua
	}
	return ua[:maxUserAgentLength]
}
