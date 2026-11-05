package orca

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

// These shapes follow upstream a19de980's public runtime-worktree-contracts,
// agent-launch-intent, agent-launch-request, and runtime-terminal-send-contract.
// Unknown/older variants fail closed rather than searching arbitrary JSON keys.
func ResolveWorktree(receipt Receipt, canonicalPath string) (string, error) {
	if !receipt.OK || receipt.Unknown || receipt.RuntimeID == "" || !filepath.IsAbs(canonicalPath) {
		return "", errors.New("workspace inventory or trusted canonical path unavailable")
	}
	var result struct {
		Worktrees []struct {
			ID           string `json:"id"`
			RepoID       string `json:"repoId"`
			Path         string `json:"path"`
			HostID       string `json:"hostId"`
			RuntimeOwner string `json:"runtimeOwnerEnvironmentId"`
			Archived     bool   `json:"isArchived"`
			Removing     bool   `json:"removing"`
		} `json:"worktrees"`
		Truncated bool `json:"truncated"`
		HostScope *struct {
			HostIDs        []string `json:"hostIds"`
			OmittedHostIDs []string `json:"omittedHostIds"`
		} `json:"hostScope"`
	}
	if json.Unmarshal(receipt.Result, &result) != nil || result.Truncated || result.HostScope == nil || !contains(result.HostScope.HostIDs, "local") || contains(result.HostScope.OmittedHostIDs, "local") {
		return "", errors.New("local workspace inventory incomplete or host scope unproven")
	}
	wanted, err := filepath.EvalSymlinks(canonicalPath)
	if err != nil {
		return "", err
	}
	wanted = filepath.Clean(wanted)
	selector := ""
	for _, row := range result.Worktrees {
		if row.HostID != "local" || row.RuntimeOwner != "" || row.Archived || row.Removing || !filepath.IsAbs(row.Path) {
			continue
		}
		actual, err := filepath.EvalSymlinks(row.Path)
		if err != nil {
			continue
		}
		actual = filepath.Clean(actual)
		match := actual == wanted
		if runtime.GOOS == "windows" {
			match = strings.EqualFold(actual, wanted)
		}
		if !match {
			continue
		}
		if !validSelector(row.ID) || row.RepoID == "" || row.ID != row.RepoID+"::"+row.Path {
			return "", errors.New("workspace exact identity inconsistent")
		}
		if selector != "" {
			return "", errors.New("multiple local Orca workspaces share this checkout; select an exact workspace explicitly")
		}
		selector = "id:" + row.ID
	}
	if selector == "" {
		return "", errors.New("no verified local Orca workspace matches this trusted project")
	}
	return selector, nil
}

type CoordinatorHandoff struct {
	Terminal       string `json:"terminal,omitempty"`
	OperationID    string `json:"operationId,omitempty"`
	DeliveryStatus string `json:"deliveryStatus"`
	Accepted       bool   `json:"accepted"`
}

func ParseCoordinatorReceipt(receipt Receipt, action string) (CoordinatorHandoff, error) {
	handoff := CoordinatorHandoff{DeliveryStatus: "unknown"}
	if action == "launch" {
		var result struct {
			OperationID string `json:"operationId"`
			Outcome     struct {
				Kind   string `json:"kind"`
				Handle string `json:"handle"`
			} `json:"outcome"`
			Prompt *struct {
				Outcome  string `json:"outcome"`
				Delivery string `json:"delivery"`
			} `json:"prompt"`
		}
		if err := json.Unmarshal(receipt.Result, &result); err == nil {
			handoff.OperationID = result.OperationID
			if result.Outcome.Kind == "terminal" && validSelector(result.Outcome.Handle) {
				handoff.Terminal = result.Outcome.Handle
			}
			if receipt.OK && !receipt.Unknown && handoff.Terminal != "" && result.Prompt != nil && result.Prompt.Delivery == "submit" {
				switch result.Prompt.Outcome {
				case "handed-to-terminal":
					handoff.Accepted = true
					handoff.DeliveryStatus = "handed-to-terminal"
				case "not-delivered":
					handoff.DeliveryStatus = "not-delivered"
				case "unconfirmed":
					handoff.DeliveryStatus = "unknown"
				}
			}
		}
		// CLI agent-launch-recovery exports details.operationId on an unknown launch.
		if handoff.OperationID == "" {
			var failure struct {
				Details struct {
					OperationID string `json:"operationId"`
				} `json:"details"`
			}
			if json.Unmarshal(receipt.Error, &failure) == nil {
				handoff.OperationID = failure.Details.OperationID
			}
		}
		if handoff.DeliveryStatus == "unknown" {
			return handoff, errors.New("coordinator launch or prompt delivery unconfirmed; reconcile original operation")
		}
		return handoff, nil
	}
	if action == "send" {
		var result struct {
			Send struct {
				Handle   string `json:"handle"`
				Accepted bool   `json:"accepted"`
				Prompt   *struct {
					RequestID string   `json:"requestId"`
					Stages    []string `json:"stages"`
					Provider  string   `json:"provider"`
				} `json:"prompt"`
			} `json:"send"`
		}
		if json.Unmarshal(receipt.Result, &result) != nil {
			return handoff, errors.New("invalid coordinator submission receipt")
		}
		if validSelector(result.Send.Handle) {
			handoff.Terminal = result.Send.Handle
		}
		if result.Send.Prompt != nil {
			handoff.OperationID = result.Send.Prompt.RequestID
		}
		if !receipt.OK || receipt.Unknown || !result.Send.Accepted || result.Send.Prompt == nil || result.Send.Prompt.Provider == "old-host" || handoff.OperationID == "" {
			return handoff, errors.New("coordinator prompt acceptance unconfirmed")
		}
		if contains(result.Send.Prompt.Stages, "input_accepted") {
			handoff.Accepted = true
			handoff.DeliveryStatus = "input_accepted"
		}
		if contains(result.Send.Prompt.Stages, "turn_started") {
			handoff.Accepted = true
			handoff.DeliveryStatus = "turn_started"
		}
		if !handoff.Accepted {
			return handoff, errors.New("coordinator prompt stage unconfirmed")
		}
		return handoff, nil
	}
	return handoff, errors.New("unsupported coordinator receipt action")
}
