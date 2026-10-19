package errors

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func CreateUserError(message string, options UserErrorOptions) *UserError {
	err := &UserError{
		message:    message,
		Level:      options.Level,
		Category:   options.Category,
		Details:    options.Details,
		Resolution: options.Resolution,
		Cause:      options.Cause,
	}

	level := slog.LevelWarn
	if err.Level == ErrorLevelFatal {
		level = slog.LevelError
	}

	attrs := []any{
		"category", CategoryName(err.Category),
		"level", LevelName(err.Level),
	}
	if len(err.Details) > 0 {
		attrs = append(attrs, "details", err.Details)
	}
	if len(err.Resolution) > 0 {
		attrs = append(attrs, "resolution", strings.Join(err.Resolution, "; "))
	}
	if err.Cause != nil {
		attrs = append(attrs, "cause", err.Cause.Error())
	}

	slog.Log(nil, level, "User error: "+message, attrs...)
	return err
}

func FormatErrorForDisplay(err error) string {
	if err == nil {
		return ""
	}
	if userErr, ok := err.(*UserError); ok {
		return formatUserError(userErr)
	}
	return formatSystemError(err)
}

func EnsureUserError(err error, defaultMessage string, options UserErrorOptions) *UserError {
	if err == nil {
		return CreateUserError(defaultMessage, options)
	}
	if userErr, ok := err.(*UserError); ok {
		return userErr
	}

	message := defaultMessage
	if err.Error() != "" {
		message = err.Error()
	}

	options.Cause = err
	return CreateUserError(message, options)
}

func formatUserError(err *UserError) string {
	message := fmt.Sprintf("Error: %s", err.message)

	if len(err.Resolution) > 0 {
		message += "\n\nTo resolve this:"
		for _, step := range err.Resolution {
			message += "\n- " + step
		}
	}

	if len(err.Details) > 0 {
		message += "\n\nDetails:"
		for key, value := range err.Details {
			formattedValue := formatDetailValue(value)
			message += fmt.Sprintf("\n%s: %s", key, formattedValue)
		}
	}

	return message
}

func formatSystemError(err error) string {
	message := fmt.Sprintf("System error: %s", err.Error())
	if os.Getenv("DEBUG") == "true" {
		message += "\n\nStack trace:\n" + fmt.Sprintf("%+v", err)
	}
	return message
}

func formatDetailValue(value any) string {
	if value == nil {
		return "<nil>"
	}
	if stringer, ok := value.(fmt.Stringer); ok {
		return stringer.String()
	}
	if data, err := json.MarshalIndent(value, "", "  "); err == nil {
		return string(data)
	}
	return fmt.Sprint(value)
}
