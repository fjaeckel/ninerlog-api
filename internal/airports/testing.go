package airports

// SetTestDB replaces the airport database for testing purposes.
// This should only be used in tests.
func SetTestDB(data map[string]AirportInfo) {
	db = data
}
