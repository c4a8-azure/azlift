package export

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RetryConfig controls the exponential backoff behaviour.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

// DefaultRetryConfig is 3 retries: delays of 2s, 4s, 8s.
var DefaultRetryConfig = RetryConfig{
	MaxAttempts: 3,
	BaseDelay:   2 * time.Second,
}

// throttlePatterns are substrings searched in aztfexport stderr/stdout that
// indicate Azure API throttling or transient errors worth retrying.
var throttlePatterns = []string{
	"429",
	"Too Many Requests",
	"RetryAfter",
	"Throttled",
	"ServiceUnavailable",
	"503",
	"timeout",
	"context deadline exceeded",
}

// permanentPatterns are errors that must NOT be retried — retrying would
// not help and would just slow down failure feedback.
var permanentPatterns = []string{
	"AuthorizationFailed",
	"InvalidAuthenticationToken",
	"ResourceNotFound",
	"ResourceGroupNotFound",
	"does not exist",
}

// azureADAuthPatterns are substrings that indicate the storage backend requires
// Azure AD authentication (shared-key access disabled on the storage account).
// When these appear in a failure, the run should be retried with ARM_USE_AZUREAD
// and ARM_STORAGE_USE_AZUREAD set to true.
var azureADAuthPatterns = []string{
	"KeyBasedAuthenticationNotPermitted",
	"Key based authentication is not permitted",
	"SharedKeyLite",
	"Shared key access is disabled",
	"AuthorizationPermissionMismatch",
	"storage: service returned error: StatusCode=403",
	"disallowSharedKeyAccess",
}

// IsAzureADAuthError returns true when the error and output indicate that the
// storage backend rejected shared-key auth and Azure AD auth should be used.
func IsAzureADAuthError(errMsg, output string) bool {
	combined := errMsg + "\n" + output
	for _, p := range azureADAuthPatterns {
		if strings.Contains(combined, p) {
			return true
		}
	}
	return false
}

// AzureADEnv are the environment variable pairs that enable Azure AD
// authentication for both the azurerm provider and its storage backend.
var AzureADEnv = []string{
	"ARM_USE_AZUREAD=true",
	"ARM_STORAGE_USE_AZUREAD=true",
}

// RetryRunner wraps a Runner with exponential backoff retry logic.
type RetryRunner struct {
	Inner  Runner
	Config RetryConfig
	// Sleep is called between retries; defaults to time.Sleep (overridable in tests).
	Sleep func(time.Duration)
}

// NewRetryRunner wraps inner with DefaultRetryConfig.
func NewRetryRunner(inner Runner) *RetryRunner {
	return &RetryRunner{
		Inner:  inner,
		Config: DefaultRetryConfig,
		Sleep:  time.Sleep,
	}
}

// Run implements Runner with retry logic.
func (r *RetryRunner) Run(ctx context.Context, args []string, logLine func(string)) error {
	var lastErr error
	delay := r.Config.BaseDelay

	for attempt := 1; attempt <= r.Config.MaxAttempts; attempt++ {
		// Collect output lines so we can inspect them for throttle signals.
		var lines []string
		wrappedLog := func(line string) {
			lines = append(lines, line)
			if logLine != nil {
				logLine(line)
			}
		}

		lastErr = r.Inner.Run(ctx, args, wrappedLog)
		if lastErr == nil {
			return nil
		}

		output := strings.Join(lines, "\n")
		errMsg := lastErr.Error()

		if isPermanent(errMsg, output) {
			return lastErr
		}

		if !isRetryable(errMsg, output) {
			return lastErr
		}

		if attempt == r.Config.MaxAttempts {
			break
		}

		if logLine != nil {
			logLine(formatRetryMsg(attempt, r.Config.MaxAttempts, delay, errMsg))
		}

		sleep := r.Sleep
		if sleep == nil {
			sleep = time.Sleep
		}
		sleep(delay)
		delay *= 2
	}

	return lastErr
}

func isRetryable(errMsg, output string) bool {
	combined := errMsg + "\n" + output
	for _, p := range throttlePatterns {
		if strings.Contains(combined, p) {
			return true
		}
	}
	return false
}

func isPermanent(errMsg, output string) bool {
	combined := errMsg + "\n" + output
	for _, p := range permanentPatterns {
		if strings.Contains(combined, p) {
			return true
		}
	}
	return false
}

func formatRetryMsg(attempt, max int, delay time.Duration, err string) string {
	return fmt.Sprintf("[retry] attempt %d of %d failed: %s — retrying in %s", attempt, max, err, delay)
}
