package registration

// Nationality mark table.
//
// Source of truth:
//
//  1. ICAO. Annex 7 Standard 3.3 requires a state to select its nationality
//     mark from the radio call-sign series the ITU allocated to it, and Annex
//     7 carries the hyphen convention. ICAO publishes the marks states have
//     actually selected: https://www.icao.int/nationality-marks
//  2. Wikipedia, "List of aircraft registration prefixes" — the practical
//     consolidation, and the easiest thing to diff against.
//
// ITU Radio Regulations Appendix 42, the "Table of International Call Sign
// Series", is a radiocommunication document and says nothing about aircraft.
// It is upstream of (1) only by the reference in Standard 3.3, and it
// allocates whole blocks — Germany holds DAA–DRZ, the United States holds AAA–
// ALZ, KAA–KZZ, NAA–NZZ and WAA–WZZ. Which slice of its block a state uses for
// aircraft (D, N) is the state's own choice, and hyphenation is not in
// Appendix 42 at all. It is therefore a cross-check that a mark falls inside
// its state's allocation, never a source for the two facts this table records.
//
// See docs/AIRCRAFT_REGISTRATIONS.md for the review procedure. Allocations
// change on the order of once every few years (a new state, a state changing
// its mark), which is why this table is vendored rather than fetched at
// runtime like the airport database.

// LastReviewed is when this table was last checked against upstream. Because
// the table is vendored, this date is the only signal that it may be stale —
// it is reported to operators via GET /admin/config and must be updated
// whenever the table is reviewed, whether or not anything changed.
const LastReviewed = "2026-08-17"

// Suffix patterns
//
// Suffix is a regular expression matched against the registration mark — the
// part after the nationality mark, with all separators removed. It is
// anchored on both ends at init.
//
// Most entries leave it empty and inherit defaultSuffix. A pattern is only
// spelled out where it does real work, which is one of two cases:
//
//   - A one-character mark (B, C, D, F, G, I, M, N, P, Z). Without a pattern
//     these swallow anything that happens to start with that letter — an
//     aircraft "type" mistakenly entered as a registration ("B738", "C172",
//     "G1000", "FNPT2") would come back hyphenated as "B-738". A pattern
//     makes those fall through unrecognised and be left alone.
//   - A two-character mark that shadows a one-character mark whose own
//     registration marks can start with a digit. "D2345" is a German glider
//     D-2345, not an Angolan D2-345; Angola's marks are three letters, so
//     pinning D2 to letters resolves it.
//
// Everything else is unambiguous under longest-prefix matching and does not
// need one.
const defaultSuffix = `[A-Z0-9]{1,5}`

// entries is the nationality mark table. Order within the slice does not
// matter for matching — matchOrder sorts by mark length, longest first — but
// it is kept sorted for review against upstream.
//
// Hyphen records the canonical notation, which is not derivable from the
// Annex 7 rule alone: the rule ("a registration mark starting with a letter
// is preceded by a hyphen") explains N12345, JA8089 and HL7747, but plenty of
// states hyphenate a numeric registration mark anyway — B-1234, RA-12345,
// CU-T1234. Hyphenation is therefore recorded per state, not computed.
var entries = []Entry{
	{Prefix: "3A", Country: "MC", CountryName: "Monaco"},
	{Prefix: "3B", Country: "MU", CountryName: "Mauritius"},
	{Prefix: "3C", Country: "GQ", CountryName: "Equatorial Guinea"},
	{Prefix: "3D", Country: "SZ", CountryName: "Eswatini"},
	{Prefix: "3X", Country: "GN", CountryName: "Guinea"},
	{Prefix: "4K", Country: "AZ", CountryName: "Azerbaijan"},
	{Prefix: "4L", Country: "GE", CountryName: "Georgia"},
	{Prefix: "4O", Country: "ME", CountryName: "Montenegro"},
	{Prefix: "4R", Country: "LK", CountryName: "Sri Lanka"},
	{Prefix: "4W", Country: "TL", CountryName: "Timor-Leste"},
	{Prefix: "4X", Country: "IL", CountryName: "Israel"},
	{Prefix: "5A", Country: "LY", CountryName: "Libya"},
	{Prefix: "5B", Country: "CY", CountryName: "Cyprus"},
	{Prefix: "5H", Country: "TZ", CountryName: "Tanzania"},
	{Prefix: "5N", Country: "NG", CountryName: "Nigeria"},
	{Prefix: "5R", Country: "MG", CountryName: "Madagascar"},
	{Prefix: "5T", Country: "MR", CountryName: "Mauritania"},
	{Prefix: "5U", Country: "NE", CountryName: "Niger"},
	{Prefix: "5V", Country: "TG", CountryName: "Togo"},
	{Prefix: "5W", Country: "WS", CountryName: "Samoa"},
	{Prefix: "5X", Country: "UG", CountryName: "Uganda"},
	{Prefix: "5Y", Country: "KE", CountryName: "Kenya"},
	{Prefix: "6O", Country: "SO", CountryName: "Somalia"},
	{Prefix: "6V", Country: "SN", CountryName: "Senegal"},
	{Prefix: "6Y", Country: "JM", CountryName: "Jamaica"},
	{Prefix: "7O", Country: "YE", CountryName: "Yemen"},
	{Prefix: "7P", Country: "LS", CountryName: "Lesotho"},
	{Prefix: "7Q", Country: "MW", CountryName: "Malawi"},
	{Prefix: "7T", Country: "DZ", CountryName: "Algeria"},
	{Prefix: "8P", Country: "BB", CountryName: "Barbados"},
	{Prefix: "8Q", Country: "MV", CountryName: "Maldives"},
	{Prefix: "8R", Country: "GY", CountryName: "Guyana"},
	{Prefix: "9A", Country: "HR", CountryName: "Croatia"},
	{Prefix: "9G", Country: "GH", CountryName: "Ghana"},
	{Prefix: "9H", Country: "MT", CountryName: "Malta"},
	{Prefix: "9J", Country: "ZM", CountryName: "Zambia"},
	{Prefix: "9K", Country: "KW", CountryName: "Kuwait"},
	{Prefix: "9L", Country: "SL", CountryName: "Sierra Leone"},
	{Prefix: "9M", Country: "MY", CountryName: "Malaysia"},
	{Prefix: "9N", Country: "NP", CountryName: "Nepal"},
	{Prefix: "9Q", Country: "CD", CountryName: "Democratic Republic of the Congo"},
	{Prefix: "9U", Country: "BI", CountryName: "Burundi"},
	{Prefix: "9V", Country: "SG", CountryName: "Singapore"},
	{Prefix: "9XR", Country: "RW", CountryName: "Rwanda"},
	{Prefix: "9Y", Country: "TT", CountryName: "Trinidad and Tobago"},
	{Prefix: "A2", Country: "BW", CountryName: "Botswana"},
	{Prefix: "A3", Country: "TO", CountryName: "Tonga"},
	{Prefix: "A5", Country: "BT", CountryName: "Bhutan"},
	{Prefix: "A6", Country: "AE", CountryName: "United Arab Emirates"},
	{Prefix: "A7", Country: "QA", CountryName: "Qatar"},
	{Prefix: "A9C", Country: "BH", CountryName: "Bahrain"},
	{Prefix: "AP", Country: "PK", CountryName: "Pakistan"},

	// One mark covers the mainland (B-1234, B-123A), Hong Kong (B-Hxx,
	// B-Kxx, B-Lxx), Macau (B-Mxx) and Taiwan (B-12345). They differ only in
	// which sub-range of the registration mark they draw from, never in the
	// notation, so splitting them would put the hyphen in the wrong place
	// ("BH-KA") for no gain.
	{Prefix: "B", Country: "CN", CountryName: "China (incl. Hong Kong, Macau, Taiwan)", Suffix: `[0-9]{4,5}|[0-9]{3}[A-Z]|[HKLM][A-Z0-9]{2,4}`},

	{Prefix: "C", Country: "CA", CountryName: "Canada", Suffix: `[A-Z]{4}`},
	{Prefix: "C2", Country: "NR", CountryName: "Nauru", Suffix: `[A-Z]{3}`},
	{Prefix: "C3", Country: "AD", CountryName: "Andorra", Suffix: `[A-Z]{2,3}`},
	{Prefix: "C5", Country: "GM", CountryName: "Gambia", Suffix: `[A-Z]{3}`},
	{Prefix: "C6", Country: "BS", CountryName: "Bahamas", Suffix: `[A-Z]{3}`},
	{Prefix: "C9", Country: "MZ", CountryName: "Mozambique", Suffix: `[A-Z]{3}`},
	{Prefix: "CC", Country: "CL", CountryName: "Chile", Suffix: `[A-Z]{3}`},
	{Prefix: "CN", Country: "MA", CountryName: "Morocco", Suffix: `[A-Z]{3}`},
	{Prefix: "CP", Country: "BO", CountryName: "Bolivia", Suffix: `[A-Z]{3}|[0-9]{4}`},
	{Prefix: "CS", Country: "PT", CountryName: "Portugal", Suffix: `[A-Z]{3}`},
	{Prefix: "CU", Country: "CU", CountryName: "Cuba", Suffix: `[A-Z][0-9]{3,4}|[A-Z]{3}`},
	{Prefix: "CX", Country: "UY", CountryName: "Uruguay", Suffix: `[A-Z]{3}`},

	// Powered aircraft take four letters (D-EABC, D-ABCD); gliders take four
	// digits (D-1234), which is what collides with D2/D4/D6 below.
	{Prefix: "D", Country: "DE", CountryName: "Germany", Suffix: `[A-Z]{4}|[0-9]{4}`},
	{Prefix: "D2", Country: "AO", CountryName: "Angola", Suffix: `[A-Z]{3}`},
	{Prefix: "D4", Country: "CV", CountryName: "Cabo Verde", Suffix: `[A-Z]{3}`},
	{Prefix: "D6", Country: "KM", CountryName: "Comoros", Suffix: `[A-Z]{3}`},
	{Prefix: "DQ", Country: "FJ", CountryName: "Fiji", Suffix: `[A-Z]{3}`},

	{Prefix: "E3", Country: "ER", CountryName: "Eritrea"},
	{Prefix: "E5", Country: "CK", CountryName: "Cook Islands"},
	{Prefix: "E7", Country: "BA", CountryName: "Bosnia and Herzegovina"},
	{Prefix: "EC", Country: "ES", CountryName: "Spain"},
	{Prefix: "EI", Country: "IE", CountryName: "Ireland"},
	{Prefix: "EJ", Country: "IE", CountryName: "Ireland"},
	{Prefix: "EK", Country: "AM", CountryName: "Armenia"},
	{Prefix: "EP", Country: "IR", CountryName: "Iran"},
	{Prefix: "ER", Country: "MD", CountryName: "Moldova"},
	{Prefix: "ES", Country: "EE", CountryName: "Estonia"},
	{Prefix: "ET", Country: "ET", CountryName: "Ethiopia"},
	{Prefix: "EW", Country: "BY", CountryName: "Belarus"},
	{Prefix: "EX", Country: "KG", CountryName: "Kyrgyzstan"},
	{Prefix: "EY", Country: "TJ", CountryName: "Tajikistan"},
	{Prefix: "EZ", Country: "TM", CountryName: "Turkmenistan"},
	{Prefix: "F", Country: "FR", CountryName: "France", Suffix: `[A-Z]{4}`},
	{Prefix: "G", Country: "GB", CountryName: "United Kingdom", Suffix: `[A-Z]{4}`},
	{Prefix: "H4", Country: "SB", CountryName: "Solomon Islands"},
	{Prefix: "HA", Country: "HU", CountryName: "Hungary"},
	{Prefix: "HB", Country: "CH", CountryName: "Switzerland and Liechtenstein"},
	{Prefix: "HC", Country: "EC", CountryName: "Ecuador"},
	{Prefix: "HH", Country: "HT", CountryName: "Haiti"},
	{Prefix: "HI", Country: "DO", CountryName: "Dominican Republic"},
	{Prefix: "HK", Country: "CO", CountryName: "Colombia"},

	// No hyphen: the registration mark is numeric (HL7747).
	{Prefix: "HL", Country: "KR", CountryName: "South Korea", NoHyphen: true, Suffix: `[0-9]{4}[A-Z]?`},

	{Prefix: "HP", Country: "PA", CountryName: "Panama"},
	{Prefix: "HR", Country: "HN", CountryName: "Honduras"},
	{Prefix: "HS", Country: "TH", CountryName: "Thailand"},
	{Prefix: "HZ", Country: "SA", CountryName: "Saudi Arabia"},
	{Prefix: "I", Country: "IT", CountryName: "Italy", Suffix: `[A-Z]{4}`},
	{Prefix: "J2", Country: "DJ", CountryName: "Djibouti"},
	{Prefix: "J3", Country: "GD", CountryName: "Grenada"},
	{Prefix: "J5", Country: "GW", CountryName: "Guinea-Bissau"},
	{Prefix: "J6", Country: "LC", CountryName: "Saint Lucia"},
	{Prefix: "J7", Country: "DM", CountryName: "Dominica"},
	{Prefix: "J8", Country: "VC", CountryName: "Saint Vincent and the Grenadines"},

	// No hyphen: the registration mark is numeric (JA8089, JA01AA).
	{Prefix: "JA", Country: "JP", CountryName: "Japan", NoHyphen: true, Suffix: `[0-9]{2,4}[A-Z]{0,2}`},

	{Prefix: "JU", Country: "MN", CountryName: "Mongolia"},
	{Prefix: "JY", Country: "JO", CountryName: "Jordan"},
	{Prefix: "LN", Country: "NO", CountryName: "Norway"},
	{Prefix: "LQ", Country: "AR", CountryName: "Argentina"},
	{Prefix: "LV", Country: "AR", CountryName: "Argentina"},
	{Prefix: "LX", Country: "LU", CountryName: "Luxembourg"},
	{Prefix: "LY", Country: "LT", CountryName: "Lithuania"},
	{Prefix: "LZ", Country: "BG", CountryName: "Bulgaria"},
	{Prefix: "M", Country: "IM", CountryName: "Isle of Man", Suffix: `[A-Z]{4}`},

	// No hyphen: the registration mark always starts with a digit (N12345,
	// N123AB). The letters I and O are not issued, but accepting them here
	// costs nothing — this table decides notation, not validity.
	{Prefix: "N", Country: "US", CountryName: "United States", NoHyphen: true, Suffix: `[0-9][0-9A-Z]{0,4}`},

	{Prefix: "OB", Country: "PE", CountryName: "Peru"},
	{Prefix: "OD", Country: "LB", CountryName: "Lebanon"},
	{Prefix: "OE", Country: "AT", CountryName: "Austria"},
	{Prefix: "OH", Country: "FI", CountryName: "Finland"},
	{Prefix: "OK", Country: "CZ", CountryName: "Czechia"},
	{Prefix: "OM", Country: "SK", CountryName: "Slovakia"},
	{Prefix: "OO", Country: "BE", CountryName: "Belgium"},
	{Prefix: "OY", Country: "DK", CountryName: "Denmark"},
	{Prefix: "P", Country: "KP", CountryName: "North Korea", Suffix: `[0-9]{3,4}`},
	{Prefix: "P2", Country: "PG", CountryName: "Papua New Guinea", Suffix: `[A-Z]{3}`},
	{Prefix: "P4", Country: "AW", CountryName: "Aruba", Suffix: `[A-Z]{3}`},
	{Prefix: "PH", Country: "NL", CountryName: "Netherlands"},
	{Prefix: "PJ", Country: "CW", CountryName: "Curaçao and Sint Maarten"},
	{Prefix: "PK", Country: "ID", CountryName: "Indonesia"},
	{Prefix: "PP", Country: "BR", CountryName: "Brazil"},
	{Prefix: "PR", Country: "BR", CountryName: "Brazil"},
	{Prefix: "PS", Country: "BR", CountryName: "Brazil"},
	{Prefix: "PT", Country: "BR", CountryName: "Brazil"},
	{Prefix: "PU", Country: "BR", CountryName: "Brazil"},
	{Prefix: "PZ", Country: "SR", CountryName: "Suriname"},
	{Prefix: "RA", Country: "RU", CountryName: "Russia"},
	{Prefix: "RDPL", Country: "LA", CountryName: "Laos"},
	{Prefix: "RF", Country: "RU", CountryName: "Russia"},
	{Prefix: "RP", Country: "PH", CountryName: "Philippines"},
	{Prefix: "S2", Country: "BD", CountryName: "Bangladesh"},
	{Prefix: "S5", Country: "SI", CountryName: "Slovenia"},
	{Prefix: "S7", Country: "SC", CountryName: "Seychelles"},
	{Prefix: "S9", Country: "ST", CountryName: "Sao Tome and Principe"},
	{Prefix: "SE", Country: "SE", CountryName: "Sweden"},
	{Prefix: "SP", Country: "PL", CountryName: "Poland"},
	{Prefix: "ST", Country: "SD", CountryName: "Sudan"},
	{Prefix: "SU", Country: "EG", CountryName: "Egypt"},
	{Prefix: "SX", Country: "GR", CountryName: "Greece"},
	{Prefix: "T3", Country: "KI", CountryName: "Kiribati"},
	{Prefix: "T7", Country: "SM", CountryName: "San Marino"},
	{Prefix: "T8A", Country: "PW", CountryName: "Palau"},
	{Prefix: "TC", Country: "TR", CountryName: "Turkey"},
	{Prefix: "TF", Country: "IS", CountryName: "Iceland"},
	{Prefix: "TG", Country: "GT", CountryName: "Guatemala"},
	{Prefix: "TI", Country: "CR", CountryName: "Costa Rica"},
	{Prefix: "TJ", Country: "CM", CountryName: "Cameroon"},
	{Prefix: "TL", Country: "CF", CountryName: "Central African Republic"},
	{Prefix: "TN", Country: "CG", CountryName: "Republic of the Congo"},
	{Prefix: "TR", Country: "GA", CountryName: "Gabon"},
	{Prefix: "TS", Country: "TN", CountryName: "Tunisia"},
	{Prefix: "TT", Country: "TD", CountryName: "Chad"},
	{Prefix: "TU", Country: "CI", CountryName: "Cote d'Ivoire"},
	{Prefix: "TY", Country: "BJ", CountryName: "Benin"},
	{Prefix: "TZ", Country: "ML", CountryName: "Mali"},
	{Prefix: "UK", Country: "UZ", CountryName: "Uzbekistan"},
	{Prefix: "UN", Country: "KZ", CountryName: "Kazakhstan"},
	{Prefix: "UP", Country: "KZ", CountryName: "Kazakhstan"},
	{Prefix: "UR", Country: "UA", CountryName: "Ukraine"},
	{Prefix: "V2", Country: "AG", CountryName: "Antigua and Barbuda"},
	{Prefix: "V3", Country: "BZ", CountryName: "Belize"},
	{Prefix: "V4", Country: "KN", CountryName: "Saint Kitts and Nevis"},
	{Prefix: "V5", Country: "NA", CountryName: "Namibia"},
	{Prefix: "V6", Country: "FM", CountryName: "Micronesia"},
	{Prefix: "V7", Country: "MH", CountryName: "Marshall Islands"},
	{Prefix: "V8", Country: "BN", CountryName: "Brunei"},
	{Prefix: "VH", Country: "AU", CountryName: "Australia"},
	{Prefix: "VN", Country: "VN", CountryName: "Vietnam"},

	// The British overseas territories share VP-/VQ- and are told apart by
	// the first letter of the registration mark (VP-B Bermuda, VP-C Cayman
	// Islands, VQ-T Turks and Caicos, …). As with B above, the split is
	// inside the registration mark, so the mark itself stays two characters.
	{Prefix: "VP", Country: "GB", CountryName: "British overseas territory"},
	{Prefix: "VQ", Country: "GB", CountryName: "British overseas territory"},

	{Prefix: "VT", Country: "IN", CountryName: "India"},
	{Prefix: "XA", Country: "MX", CountryName: "Mexico"},
	{Prefix: "XB", Country: "MX", CountryName: "Mexico"},
	{Prefix: "XC", Country: "MX", CountryName: "Mexico"},
	{Prefix: "XT", Country: "BF", CountryName: "Burkina Faso"},
	{Prefix: "XU", Country: "KH", CountryName: "Cambodia"},
	{Prefix: "XY", Country: "MM", CountryName: "Myanmar"},
	{Prefix: "YA", Country: "AF", CountryName: "Afghanistan"},
	{Prefix: "YI", Country: "IQ", CountryName: "Iraq"},
	{Prefix: "YJ", Country: "VU", CountryName: "Vanuatu"},
	{Prefix: "YK", Country: "SY", CountryName: "Syria"},
	{Prefix: "YL", Country: "LV", CountryName: "Latvia"},
	{Prefix: "YN", Country: "NI", CountryName: "Nicaragua"},
	{Prefix: "YR", Country: "RO", CountryName: "Romania"},
	{Prefix: "YS", Country: "SV", CountryName: "El Salvador"},
	{Prefix: "YU", Country: "RS", CountryName: "Serbia"},
	{Prefix: "YV", Country: "VE", CountryName: "Venezuela"},
	{Prefix: "Z", Country: "ZW", CountryName: "Zimbabwe", Suffix: `[A-Z]{3}`},
	{Prefix: "Z3", Country: "MK", CountryName: "North Macedonia", Suffix: `[A-Z]{3}`},
	{Prefix: "ZA", Country: "AL", CountryName: "Albania", Suffix: `[A-Z]{3}`},
	{Prefix: "ZK", Country: "NZ", CountryName: "New Zealand", Suffix: `[A-Z]{3}`},
	{Prefix: "ZL", Country: "NZ", CountryName: "New Zealand", Suffix: `[A-Z]{3}`},
	{Prefix: "ZM", Country: "NZ", CountryName: "New Zealand", Suffix: `[A-Z]{3}`},
	{Prefix: "ZP", Country: "PY", CountryName: "Paraguay", Suffix: `[A-Z]{3}`},
	{Prefix: "ZS", Country: "ZA", CountryName: "South Africa", Suffix: `[A-Z]{3}`},
	{Prefix: "ZT", Country: "ZA", CountryName: "South Africa", Suffix: `[A-Z]{3}`},
	{Prefix: "ZU", Country: "ZA", CountryName: "South Africa", Suffix: `[A-Z]{3}`},
}
