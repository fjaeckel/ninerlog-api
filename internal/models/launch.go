package models

// Launch methods of a flight (SFCL.155).
const (
	LaunchWinch      = "winch"
	LaunchAerotow    = "aerotow"
	LaunchSelfLaunch = "self-launch"
	LaunchCar        = "car"
	LaunchBungee     = "bungee"
)

// TowedLaunchMethods returns the launch methods that do not count toward
// powered classes.
func TowedLaunchMethods() []string {
	return []string{LaunchWinch, LaunchAerotow, LaunchCar, LaunchBungee}
}

// IsTowedLaunch reports whether method is a towed launch method.
func IsTowedLaunch(method *string) bool {
	if method == nil {
		return false
	}
	for _, m := range TowedLaunchMethods() {
		if *method == m {
			return true
		}
	}
	return false
}
