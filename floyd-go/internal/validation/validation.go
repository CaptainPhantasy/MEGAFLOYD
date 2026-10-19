package validation

import (
	"encoding/json"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

var pathPattern = regexp.MustCompile(`^[a-zA-Z0-9/\\._\-~]+$`)

func IsNonEmptyString(value string) bool {
	return strings.TrimSpace(value) != ""
}

func IsEmail(value string) bool {
	_, err := mail.ParseAddress(value)
	return err == nil
}

func IsURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func IsValidPath(value string) bool {
	if value == "" {
		return false
	}
	if strings.Contains(value, "..") {
		return false
	}
	return pathPattern.MatchString(value)
}

func IsValidFilePath(value string) bool {
	if !IsValidPath(value) {
		return false
	}
	return !strings.HasSuffix(value, "/") && !strings.HasSuffix(value, "\\")
}

func IsValidDirectoryPath(value string) bool {
	return IsValidPath(value)
}

func IsValidJSON(value string) bool {
	var data any
	return json.Unmarshal([]byte(value), &data) == nil
}
