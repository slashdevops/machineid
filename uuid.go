package machineid

import "uuid"

// isValidUUID reports whether s is a well-formed UUID that is neither the nil
// UUID (all zeros) nor the max UUID (all ones). Firmware with no UUID
// programmed commonly reports one of those two sentinels, and both must be
// rejected so they cannot contribute a machine-independent value to the ID.
//
// Only the check is strict; callers keep hashing the raw string exactly as
// the platform reported it, so accepted values produce the same ID as before.
func isValidUUID(s string) bool {
	u, err := uuid.Parse(s)
	if err != nil {
		return false
	}

	return u != uuid.Nil() && u != uuid.Max()
}
