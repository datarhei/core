package letsdebug

import (
	"fmt"
	"strings"
)

// SeverityLevel represents the priority of a reported problem
type SeverityLevel string

// Problem represents an issue found by one of the checkers in this package.
// Explanation is a human-readable explanation of the issue.
// Detail is usually the underlying machine error.
type Problem struct {
	Name        string        `json:"name"`
	Explanation string        `json:"explanation"`
	Detail      string        `json:"detail"`
	Severity    SeverityLevel `json:"severity"`
}

const (
	SeverityFatal   SeverityLevel = "Fatal" // Represents a fatal error which will stop any further checks
	SeverityError   SeverityLevel = "Error"
	SeverityWarning SeverityLevel = "Warning"
	SeverityDebug   SeverityLevel = "Debug" // Not to be shown by default
)

func (p Problem) String() string {
	return fmt.Sprintf("[%s] %s: %s", p.Name, p.Explanation, p.Detail)
}

func (p Problem) IsZero() bool {
	return p.Name == ""
}

func (p Problem) DetailLines() []string {
	return strings.Split(p.Detail, "\n")
}

func hasFatalProblem(probs []Problem) bool {
	for _, p := range probs {
		if p.Severity == SeverityFatal {
			return true
		}
	}

	return false
}

func internalProblem(message string, level SeverityLevel) Problem {
	return Problem{
		Name:        "InternalProblem",
		Explanation: "An internal error occurred while checking the domain",
		Detail:      message,
		Severity:    level,
	}
}

func dnsLookupFailed(name, rrType string, err error) Problem {
	return Problem{
		Name:        "DNSLookupFailed",
		Explanation: fmt.Sprintf(`A fatal issue occurred during the DNS lookup process for %s/%s.`, name, rrType),
		Detail:      err.Error(),
		Severity:    SeverityFatal,
	}
}

func debugProblem(name, message, detail string) Problem {
	return Problem{
		Name:        name,
		Explanation: message,
		Detail:      detail,
		Severity:    SeverityDebug,
	}
}
