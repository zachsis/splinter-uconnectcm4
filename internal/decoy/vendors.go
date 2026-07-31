package decoy

// Vendor pairs a Bluetooth SIG Company Identifier with an optional short product
// name. The company ID is the spec-defined vendor signal a scanner reads from
// manufacturer-specific data; it does NOT trigger pairing popups.
type Vendor struct {
	CompanyID uint16 // Bluetooth SIG company identifier (little-endian on air)
	Name      string // "" = nameless; otherwise <= 12 chars (31-byte adv budget)
}

// Company IDs / service UUIDs deliberately NEVER emitted: their well-known
// payload formats pop pairing dialogs on bystanders' devices. Shared with the
// parity harness so both sides assert the same exclusions.
const (
	CompanyApple          uint16 = 0x004C // Apple Continuity
	CompanyMicrosoft      uint16 = 0x0006 // Microsoft Swift Pair
	ServiceGoogleFastPair uint16 = 0xFE2C // Google Fast Pair (service data)
)

// Vendors is an independently-curated palette of real Bluetooth SIG company
// identifiers (from the public assigned-numbers registry) with plausible
// consumer-audio/wearable names. Extend from the registry for a denser crowd:
//
//	https://www.bluetooth.com/specifications/assigned-numbers/
//
// Deliberately excludes CompanyApple, CompanyMicrosoft, and never uses the
// Google Fast Pair service-data shape.
var Vendors = []Vendor{
	{0x0075, "Galaxy Buds"}, // Samsung Electronics
	{0x00E0, "Pixel Buds"},  // Google
	{0x009E, "SoundLink"},   // Bose Corporation
	{0x0087, "Forerunner"},  // Garmin International
	{0x012D, "LinkBuds"},    // Sony Corporation
	{0x0157, "Amazfit"},     // Anhui Huami
	{0x0059, ""},            // Nordic Semiconductor (nameless sensor)
	{0x0171, ""},            // Amazon.com Services (nameless)
	{0x0075, ""},            // Samsung (nameless wearable)
	{0x0087, "vivo band"},   // Garmin (band)
}
