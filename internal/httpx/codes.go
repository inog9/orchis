package httpx

import "strings"

// Codes are stable UI contracts. Unrecognized server detail stays diagnostic;
// clients display a localized status fallback instead of rendering raw text.
func problemCode(status int, detail string) string {
	if code := knownMessageCode(detail); code != "" {
		return code
	}
	switch status {
	case 400:
		return "invalid_request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 409:
		return "conflict"
	case 413:
		return "payload_too_large"
	case 422:
		return "validation_failed"
	case 429:
		return "rate_limited"
	default:
		return "request_failed"
	}
}
func knownMessageCode(message string) string {
	if strings.HasPrefix(message, "Dependencies form a loop:") {
		return "dependency_cycle"
	}
	if strings.HasPrefix(message, "Step ") && strings.HasSuffix(message, " has no agent. Install one from Assets → Agents first.") {
		return "step_asset_no_agent"
	}
	switch message {
	case "Name is required (max 200 characters).":
		return "name_invalid"
	case "The file is empty.":
		return "file_empty"
	case "Use ports like 22,80,8000-8100 (at most 50 entries).":
		return "ports_invalid"
	case "This asset is not authorized for testing.":
		return "testing_not_authorized"
	case "The agent on this asset is offline.":
		return "agent_offline"
	case "Collection choices changed; reload before saving.":
		return "collection_conflict"
	case "This asset was changed by someone else. Reload and try again.":
		return "asset_conflict"
	case "Already running.":
		return "already_running"
	case "Choose English or Indonesia.":
		return "language_invalid"
	case "Enter a valid email address.":
		return "email_invalid"
	case "This asset has no agent. Install one from Assets → Agents first.":
		return "asset_no_agent"
	case "Email or password is incorrect.":
		return "invalid_credentials"
	case "This server has no agent release to install. An administrator must publish one first.":
		return "release_unavailable"
	case "This agent is already revoked.":
		return "agent_revoked"
	case "An account with this email already exists.":
		return "account_email_exists"
	case "You can’t change your own role or status. Ask another administrator.":
		return "own_role_change"
	case "Use at least 12 characters.":
		return "password_too_short"
	case "That isn’t your current password.":
		return "current_password_wrong"
	case "Someone with this email already exists.":
		return "person_email_exists"
	}
	return ""
}
