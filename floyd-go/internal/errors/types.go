package errors

import "fmt"

type ErrorLevel int

const (
	ErrorLevelCritical ErrorLevel = iota
	ErrorLevelMajor
	ErrorLevelMinor
	ErrorLevelInformational
	ErrorLevelDebug
	ErrorLevelInfo
	ErrorLevelWarning
	ErrorLevelError
	ErrorLevelFatal
)

type ErrorCategory int

const (
	ErrorCategoryApplication ErrorCategory = iota
	ErrorCategoryAuthentication
	ErrorCategoryNetwork
	ErrorCategoryFileSystem
	ErrorCategoryCommandExecution
	ErrorCategoryAIService
	ErrorCategoryConfiguration
	ErrorCategoryResource
	ErrorCategoryUnknown
	ErrorCategoryInternal
	ErrorCategoryValidation
	ErrorCategoryInitialization
	ErrorCategoryServer
	ErrorCategoryAPI
	ErrorCategoryTimeout
	ErrorCategoryRateLimit
	ErrorCategoryConnection
	ErrorCategoryAuthorization
	ErrorCategoryFileNotFound
	ErrorCategoryFileAccess
	ErrorCategoryFileRead
	ErrorCategoryFileWrite
	ErrorCategoryCommand
	ErrorCategoryCommandNotFound
)

type UserErrorOptions struct {
	Level      ErrorLevel
	Category   ErrorCategory
	Details    map[string]any
	Resolution []string
	Cause      error
}

type UserError struct {
	message    string
	Level      ErrorLevel
	Category   ErrorCategory
	Details    map[string]any
	Resolution []string
	Cause      error
}

func (e *UserError) Error() string {
	return e.message
}

func (e *UserError) Unwrap() error {
	return e.Cause
}

func (e *UserError) Message() string {
	return e.message
}

func (e *UserError) String() string {
	return fmt.Sprintf("%s (category=%s)", e.message, CategoryName(e.Category))
}

func CategoryName(category ErrorCategory) string {
	switch category {
	case ErrorCategoryApplication:
		return "Application"
	case ErrorCategoryAuthentication:
		return "Authentication"
	case ErrorCategoryNetwork:
		return "Network"
	case ErrorCategoryFileSystem:
		return "FileSystem"
	case ErrorCategoryCommandExecution:
		return "CommandExecution"
	case ErrorCategoryAIService:
		return "AIService"
	case ErrorCategoryConfiguration:
		return "Configuration"
	case ErrorCategoryResource:
		return "Resource"
	case ErrorCategoryUnknown:
		return "Unknown"
	case ErrorCategoryInternal:
		return "Internal"
	case ErrorCategoryValidation:
		return "Validation"
	case ErrorCategoryInitialization:
		return "Initialization"
	case ErrorCategoryServer:
		return "Server"
	case ErrorCategoryAPI:
		return "API"
	case ErrorCategoryTimeout:
		return "Timeout"
	case ErrorCategoryRateLimit:
		return "RateLimit"
	case ErrorCategoryConnection:
		return "Connection"
	case ErrorCategoryAuthorization:
		return "Authorization"
	case ErrorCategoryFileNotFound:
		return "FileNotFound"
	case ErrorCategoryFileAccess:
		return "FileAccess"
	case ErrorCategoryFileRead:
		return "FileRead"
	case ErrorCategoryFileWrite:
		return "FileWrite"
	case ErrorCategoryCommand:
		return "Command"
	case ErrorCategoryCommandNotFound:
		return "CommandNotFound"
	default:
		return "Unknown"
	}
}

func LevelName(level ErrorLevel) string {
	switch level {
	case ErrorLevelCritical:
		return "Critical"
	case ErrorLevelMajor:
		return "Major"
	case ErrorLevelMinor:
		return "Minor"
	case ErrorLevelInformational:
		return "Informational"
	case ErrorLevelDebug:
		return "Debug"
	case ErrorLevelInfo:
		return "Info"
	case ErrorLevelWarning:
		return "Warning"
	case ErrorLevelError:
		return "Error"
	case ErrorLevelFatal:
		return "Fatal"
	default:
		return "Unknown"
	}
}
