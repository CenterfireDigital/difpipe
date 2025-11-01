package core

// Exit codes for semantic error handling
// These codes help AI agents and scripts understand error types
const (
	// ExitSuccess indicates successful completion
	ExitSuccess = 0

	// ExitGeneralError indicates a general error
	ExitGeneralError = 1

	// ExitConfigError indicates configuration error (invalid config, missing fields)
	ExitConfigError = 10

	// ExitAuthError indicates authentication failure (invalid credentials, expired tokens)
	ExitAuthError = 11

	// ExitNetworkError indicates network-related error (retryable)
	ExitNetworkError = 12

	// ExitSourceNotFound indicates source path doesn't exist
	ExitSourceNotFound = 20

	// ExitDestNotWritable indicates destination is not writable
	ExitDestNotWritable = 21

	// ExitPermissionDenied indicates permission denied error
	ExitPermissionDenied = 22

	// ExitInsufficientSpace indicates insufficient disk space at destination
	ExitInsufficientSpace = 23

	// ExitTransferFailed indicates transfer failed (retryable)
	ExitTransferFailed = 30

	// ExitChecksumMismatch indicates checksum/integrity check failed
	ExitChecksumMismatch = 31

	// ExitPartialTransfer indicates partial transfer (some files failed)
	ExitPartialTransfer = 32

	// ExitEngineNotFound indicates requested transfer engine not available
	ExitEngineNotFound = 40

	// ExitUnsupportedProtocol indicates protocol not supported by chosen engine
	ExitUnsupportedProtocol = 41

	// ExitInvalidStrategy indicates invalid strategy specified
	ExitInvalidStrategy = 42

	// ExitUserCanceled indicates user canceled the operation
	ExitUserCanceled = 50

	// ExitTimeout indicates operation timed out
	ExitTimeout = 51

	// ExitQuotaExceeded indicates quota/rate limit exceeded
	ExitQuotaExceeded = 52
)

// ErrorCategory classifies errors for agent decision-making
type ErrorCategory string

const (
	// CategoryRetryable errors can be retried
	CategoryRetryable ErrorCategory = "retryable"

	// CategoryFatal errors cannot be retried without fixing the issue
	CategoryFatal ErrorCategory = "fatal"

	// CategoryConfiguration errors require config changes
	CategoryConfiguration ErrorCategory = "configuration"

	// CategoryAuth errors require authentication changes
	CategoryAuth ErrorCategory = "auth"

	// CategoryResource errors indicate resource constraints
	CategoryResource ErrorCategory = "resource"

	// CategoryUser errors caused by user action/cancellation
	CategoryUser ErrorCategory = "user"
)

// ExitCodeInfo provides metadata about exit codes
type ExitCodeInfo struct {
	Code        int
	Category    ErrorCategory
	Description string
	Retryable   bool
	Suggestion  string
}

// ExitCodeRegistry maps exit codes to their metadata
var ExitCodeRegistry = map[int]ExitCodeInfo{
	ExitSuccess: {
		Code:        ExitSuccess,
		Category:    CategoryUser,
		Description: "Operation completed successfully",
		Retryable:   false,
		Suggestion:  "",
	},
	ExitConfigError: {
		Code:        ExitConfigError,
		Category:    CategoryConfiguration,
		Description: "Configuration error",
		Retryable:   false,
		Suggestion:  "Check configuration file syntax and required fields",
	},
	ExitAuthError: {
		Code:        ExitAuthError,
		Category:    CategoryAuth,
		Description: "Authentication failed",
		Retryable:   false,
		Suggestion:  "Verify credentials and access permissions",
	},
	ExitNetworkError: {
		Code:        ExitNetworkError,
		Category:    CategoryRetryable,
		Description: "Network error",
		Retryable:   true,
		Suggestion:  "Check network connectivity and retry",
	},
	ExitSourceNotFound: {
		Code:        ExitSourceNotFound,
		Category:    CategoryFatal,
		Description: "Source path not found",
		Retryable:   false,
		Suggestion:  "Verify source path exists and is accessible",
	},
	ExitDestNotWritable: {
		Code:        ExitDestNotWritable,
		Category:    CategoryFatal,
		Description: "Destination not writable",
		Retryable:   false,
		Suggestion:  "Check destination permissions and path validity",
	},
	ExitPermissionDenied: {
		Code:        ExitPermissionDenied,
		Category:    CategoryAuth,
		Description: "Permission denied",
		Retryable:   false,
		Suggestion:  "Verify user has required permissions",
	},
	ExitInsufficientSpace: {
		Code:        ExitInsufficientSpace,
		Category:    CategoryResource,
		Description: "Insufficient disk space",
		Retryable:   false,
		Suggestion:  "Free up space at destination or use different location",
	},
	ExitTransferFailed: {
		Code:        ExitTransferFailed,
		Category:    CategoryRetryable,
		Description: "Transfer failed",
		Retryable:   true,
		Suggestion:  "Retry with checkpoint enabled to resume from failure point",
	},
	ExitChecksumMismatch: {
		Code:        ExitChecksumMismatch,
		Category:    CategoryFatal,
		Description: "Checksum verification failed",
		Retryable:   true,
		Suggestion:  "Data corruption detected, retry transfer",
	},
	ExitPartialTransfer: {
		Code:        ExitPartialTransfer,
		Category:    CategoryRetryable,
		Description: "Partial transfer (some files failed)",
		Retryable:   true,
		Suggestion:  "Review failed files and retry",
	},
	ExitEngineNotFound: {
		Code:        ExitEngineNotFound,
		Category:    CategoryConfiguration,
		Description: "Transfer engine not found",
		Retryable:   false,
		Suggestion:  "Install required transfer engine (rclone, rsync, etc.)",
	},
	ExitUnsupportedProtocol: {
		Code:        ExitUnsupportedProtocol,
		Category:    CategoryConfiguration,
		Description: "Protocol not supported",
		Retryable:   false,
		Suggestion:  "Use different strategy or install engine with protocol support",
	},
	ExitUserCanceled: {
		Code:        ExitUserCanceled,
		Category:    CategoryUser,
		Description: "Operation canceled by user",
		Retryable:   false,
		Suggestion:  "",
	},
	ExitTimeout: {
		Code:        ExitTimeout,
		Category:    CategoryRetryable,
		Description: "Operation timed out",
		Retryable:   true,
		Suggestion:  "Increase timeout or check for hanging processes",
	},
	ExitQuotaExceeded: {
		Code:        ExitQuotaExceeded,
		Category:    CategoryResource,
		Description: "Quota or rate limit exceeded",
		Retryable:   true,
		Suggestion:  "Wait for quota reset or increase limits",
	},
}

// GetExitCodeInfo retrieves metadata for an exit code
func GetExitCodeInfo(code int) ExitCodeInfo {
	if info, exists := ExitCodeRegistry[code]; exists {
		return info
	}
	return ExitCodeInfo{
		Code:        code,
		Category:    CategoryFatal,
		Description: "Unknown error",
		Retryable:   false,
		Suggestion:  "Check logs for details",
	}
}

// IsRetryable checks if an exit code represents a retryable error
func IsRetryable(code int) bool {
	return GetExitCodeInfo(code).Retryable
}
