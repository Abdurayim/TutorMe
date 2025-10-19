package utils

import (
	"fmt"

	"github.com/nyaruka/phonenumbers"
)

// ValidateAndFormatPhoneNumber parses an international phone number and returns it in E.164 format.
// It expects the input string to be in an international format (e.g., starting with '+').
func ValidateAndFormatPhoneNumber(phoneStr string) (string, error) {
	// The second argument is the default region. We pass an empty string ""
	// because we expect Telegram to always provide the number in a full
	// international format (e.g., +14155552671) when a user shares their contact.
	num, err := phonenumbers.Parse(phoneStr, "")
	if err != nil {
		return "", fmt.Errorf("could not parse phone number: %v", err)
	}

	// We still check if the number is valid according to the library's rules.
	if !phonenumbers.IsValidNumber(num) {
		return "", fmt.Errorf("invalid phone number")
	}

	// Format the number into the standard E.164 format to ensure consistency.
	return phonenumbers.Format(num, phonenumbers.E164), nil
}
