package logging

// Redact masks all but the last 4 characters with asterisks.
// Example: "021000021" → "*****0021", "12" → "**12", "" → "".
func Redact(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 4 {
		masked := make([]byte, len(s))
		for i := range masked {
			masked[i] = '*'
		}
		return string(masked)
	}
	masked := make([]byte, len(s))
	for i := 0; i < len(s)-4; i++ {
		masked[i] = '*'
	}
	copy(masked[len(s)-4:], s[len(s)-4:])
	return string(masked)
}
