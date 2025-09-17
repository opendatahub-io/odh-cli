package doctor

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Status string

const (
	StatusOK      Status = "OK"
	StatusWarning Status = "WARNING"
	StatusError   Status = "ERROR"
)

type Category struct {
	Name    string  `json:"name"`
	Status  Status  `json:"status"`
	Message string  `json:"message"`
	Checks  []Check `json:"checks,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

type DiagnosticCheck interface {
	Execute(ctx context.Context, client client.Client) []Category
}

type Summary struct {
	OK      int `json:"ok"`
	Warning int `json:"warning"`
	Error   int `json:"error"`
}

type CheckResults struct {
	Categories []Category `json:"categories"`
	Summary    Summary    `json:"summary"`
}

// ComputeStatus determines the overall status of a category based on its checks
// Priority: ERROR > WARNING > OK
func ComputeStatus(c Category) Status {
	if len(c.Checks) == 0 {
		return c.Status
	}

	hasWarning := false

	for _, check := range c.Checks {
		switch check.Status {
		case StatusError:
			// Return immediately on first error
			return StatusError
		case StatusWarning:
			hasWarning = true
		}
	}

	if hasWarning {
		return StatusWarning
	}

	return StatusOK
}

func ComputeSummary(c Category) Summary {
	summary := Summary{}

	// If category has individual checks, count only the checks (category
	// status is computed from checks, so counting both would be double-counting)
	if len(c.Checks) > 0 {
		for _, check := range c.Checks {
			switch check.Status {
			case StatusOK:
				summary.OK++
			case StatusWarning:
				summary.Warning++
			case StatusError:
				summary.Error++
			}
		}
	} else {
		// If no individual checks, count the category itself
		switch c.Status {
		case StatusOK:
			summary.OK++
		case StatusWarning:
			summary.Warning++
		case StatusError:
			summary.Error++
		}
	}

	return summary
}
