package decoy

import "fmt"

// CompanyLabel maps a Bluetooth SIG company ID to a friendly vendor name for
// display, falling back to the hex ID for unknown vendors.
func CompanyLabel(id uint16) string {
	switch id {
	case CompanyApple:
		return "Apple"
	case 0x0075:
		return "Samsung"
	case 0x00E0:
		return "Google"
	case 0x009E:
		return "Bose"
	case 0x0087:
		return "Garmin"
	case 0x012D:
		return "Sony"
	case 0x0157:
		return "Huami"
	case 0x0059:
		return "Nordic"
	case 0x0171:
		return "Amazon"
	case 0x038F:
		return "Xiaomi"
	case 0x0499:
		return "Ruuvi"
	case 0x0131:
		return "Cypress"
	default:
		return fmt.Sprintf("%#04x", id)
	}
}

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
// identifiers (from the public assigned-numbers registry) paired with plausible
// product-style names, spread across device categories so the decoy crowd
// resembles a real environment. The company IDs are facts from the registry;
// extend/verify against it for an even denser crowd:
//
//	https://www.bluetooth.com/specifications/assigned-numbers/
//
// Deliberately excludes CompanyApple, CompanyMicrosoft, and never uses the
// Google Fast Pair service-data shape. Names are ≤ 12 chars; a mix are nameless.
var Vendors = []Vendor{
	// --- audio: earbuds / headphones / speakers ---
	{0x0075, "Galaxy Buds"}, // Samsung Electronics
	{0x00E0, "Pixel Buds"},  // Google
	{0x009E, "SoundLink"},   // Bose Corporation
	{0x009E, "QC Ultra"},    // Bose (headphones)
	{0x012D, "LinkBuds"},    // Sony Corporation
	{0x012D, "WH-1000"},     // Sony (headphones)
	{0x0171, "Echo Buds"},   // Amazon.com Services

	// --- watches / fitness bands ---
	{0x0087, "Forerunner"},  // Garmin International
	{0x0087, "Venu"},        // Garmin (watch)
	{0x0157, "Amazfit"},     // Anhui Huami
	{0x0157, "Zepp"},        // Anhui Huami (band)
	{0x038F, "Mi Band"},     // Xiaomi
	{0x038F, "Redmi Watch"}, // Xiaomi

	// --- item trackers / tags / sensors ---
	{0x0059, ""},         // Nordic Semiconductor (nameless sensor)
	{0x0059, "Beacon"},   // Nordic (beacon)
	{0x0499, "RuuviTag"}, // Ruuvi Innovations
	{0x0131, ""},         // Cypress/Infineon (nameless tag)
	{0x0171, ""},         // Amazon (nameless)

	// --- phones / tablets ---
	{0x0075, "Galaxy S"}, // Samsung
	{0x00E0, "Pixel"},    // Google
	{0x038F, "Redmi"},    // Xiaomi

	// --- car / automotive BLE ---
	{0x0087, "Drive"}, // Garmin (automotive nav)
	{0x0075, ""},      // Samsung (nameless car kit)

	// --- laptops / peripherals / dev boards ---
	{0x00E0, ""},      // Google (nameless)
	{0x012D, ""},      // Sony (nameless)
	{0x0059, "nRF52"}, // Nordic (dev board)
}
