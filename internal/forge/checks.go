// SPDX-License-Identifier: Apache-2.0

package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Check is one CI check on a pull request.
type Check struct {
	Name string `json:"name"`
	// Bucket is "pass", "fail", "pending", "skipping" or "cancel".
	Bucket string `json:"bucket"`
	// Link is the check's https page, or empty.
	Link string `json:"link,omitempty"`
}

// maxChecks caps how many checks one pull request reports.
const maxChecks = 100

// Checks returns the CI checks on the pull request at prURL. A pull request
// without checks returns an empty list.
func (g GitHub) Checks(ctx context.Context, dir, prURL string) ([]Check, error) {
	if _, err := checkURL(prURL); err != nil {
		return nil, err
	}
	// gh pr checks exits 1 when a check failed and 8 while one is pending,
	// and still prints the JSON. So read stdout whatever the exit code.
	out, err := g.run(ctx, dir, "", "pr", "checks", "--json=name,bucket,link", "--", prURL)
	if err != nil && strings.Contains(err.Error(), "no checks reported") {
		return []Check{}, nil
	}
	var list []Check
	if jsonErr := json.Unmarshal([]byte(out), &list); jsonErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("gh pr checks: bad output: %w", jsonErr)
	}
	if len(list) > maxChecks {
		list = list[:maxChecks]
	}
	for i := range list {
		// Only https links reach the UI.
		if _, err := checkURL(list[i].Link); err != nil {
			list[i].Link = ""
		}
		switch list[i].Bucket {
		case "pass", "fail", "pending", "skipping", "cancel":
		default:
			list[i].Bucket = "pending"
		}
	}
	return list, nil
}
