package utils

import (
	"fmt"
	"time"
)

// FormatBytes formats bytes into human-readable size
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ParseBytes parses human-readable size to bytes
func ParseBytes(s string) (int64, error) {
	var value float64
	var unit string
	_, err := fmt.Sscanf(s, "%f%s", &value, &unit)
	if err != nil {
		return 0, fmt.Errorf("invalid size format: %w", err)
	}

	multiplier := int64(1)
	switch unit {
	case "B":
		multiplier = 1
	case "KB", "K":
		multiplier = 1024
	case "MB", "M":
		multiplier = 1024 * 1024
	case "GB", "G":
		multiplier = 1024 * 1024 * 1024
	case "TB", "T":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}

	return int64(value * float64(multiplier)), nil
}

// FormatDuration formats duration into human-readable string
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	days := d.Hours() / 24
	return fmt.Sprintf("%.1fd", days)
}

// FormatTimestamp formats timestamp for display
func FormatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// FormatPercentage formats a ratio as percentage
func FormatPercentage(numerator, denominator int64) string {
	if denominator == 0 {
		return "0.0%"
	}
	percentage := (float64(numerator) / float64(denominator)) * 100
	return fmt.Sprintf("%.1f%%", percentage)
}
