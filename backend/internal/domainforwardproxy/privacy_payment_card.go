package domainforwardproxy

// looksLikePaymentCard requires plausible issuer digits and a valid Luhn
// checksum. It does not validate card ownership or network authenticity.
// No original card digits are persisted, logged, or exported.
func looksLikePaymentCard(raw string) bool {
	digits := make([]byte, 0, 19)
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; {
		case c >= '0' && c <= '9':
			digits = append(digits, c-'0')
		case c == ' ' || c == '-':
		default:
			return false
		}
	}
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	first := int(digits[0])
	second := int(digits[1])
	issuer := first == 4 || first == 6 ||
		(first == 5 && second >= 1 && second <= 5) ||
		(first == 3 && (second == 4 || second == 7)) ||
		(first == 2 && second >= 2 && second <= 7)
	if !issuer {
		return false
	}
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		digit := int(digits[i])
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}
