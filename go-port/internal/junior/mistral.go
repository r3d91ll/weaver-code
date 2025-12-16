// Mistral tool call format parser for Devstral models.
//
// Devstral models use Mistral's native tool calling format:
//   [TOOL_CALLS]function_name[ARGS]{"param": "value"}
//
// This differs from OpenAI's format which uses structured JSON.
// This parser converts Mistral format to OpenAI-compatible ToolCall structs.
package junior

import (
	"fmt"
	"regexp"
	"strings"
)

// MistralToolCallMarker is the prefix that indicates Mistral tool call format.
const MistralToolCallMarker = "[TOOL_CALLS]"

// MistralArgsMarker separates function name from arguments.
const MistralArgsMarker = "[ARGS]"

// mistralToolCallRegex matches Mistral tool call format.
// Format: [TOOL_CALLS]function_name[ARGS]{json_args}
// Can have multiple tool calls, each starting with [TOOL_CALLS]
var mistralToolCallRegex = regexp.MustCompile(`\[TOOL_CALLS\](\w+)\[ARGS\](\{[^}]*\}|\{[\s\S]*?\n\})`)

// IsMistralToolCallFormat checks if content contains Mistral tool call format.
func IsMistralToolCallFormat(content string) bool {
	return strings.Contains(content, MistralToolCallMarker)
}

// ParseMistralToolCalls extracts tool calls from Mistral format.
// Returns parsed tool calls and any remaining text content.
//
// Example input:
//
//	[TOOL_CALLS]write_file[ARGS]{"path": "test.py", "content": "print('hello')"}
//
// Example output:
//
//	ToolCalls: [{ID: "mistral-0", Type: "function", Function: {Name: "write_file", Arguments: "{...}"}}]
//	RemainingContent: ""
func ParseMistralToolCalls(content string) ([]ToolCall, string) {
	if !IsMistralToolCallFormat(content) {
		return nil, content
	}

	var toolCalls []ToolCall
	var callID int

	// Find all tool calls in the content
	// We need a more robust parser since JSON can span multiple lines
	remaining := content
	for {
		startIdx := strings.Index(remaining, MistralToolCallMarker)
		if startIdx == -1 {
			break
		}

		// Extract text before the tool call
		beforeToolCall := remaining[:startIdx]
		remaining = remaining[startIdx+len(MistralToolCallMarker):]

		// Find function name (everything until [ARGS])
		argsIdx := strings.Index(remaining, MistralArgsMarker)
		if argsIdx == -1 {
			// Malformed - no [ARGS] marker
			break
		}

		funcName := strings.TrimSpace(remaining[:argsIdx])
		remaining = remaining[argsIdx+len(MistralArgsMarker):]

		// Extract JSON arguments
		// Handle both single-line and multi-line JSON
		jsonArgs, rest, ok := extractJSON(remaining)
		if !ok {
			// Malformed JSON or truncated
			// Try to use what we have
			jsonArgs = remaining
			rest = ""
		}

		toolCall := ToolCall{
			ID:   fmt.Sprintf("mistral-%d", callID),
			Type: "function",
			Function: ToolFunction{
				Name:      funcName,
				Arguments: jsonArgs,
			},
		}
		toolCalls = append(toolCalls, toolCall)
		callID++

		// Keep any text before the tool call as remaining content
		if beforeToolCall != "" {
			// For now, we discard pre-tool-call text
			// Could be accumulated if needed
		}

		remaining = rest
	}

	// Trim whitespace from remaining content
	remaining = strings.TrimSpace(remaining)

	return toolCalls, remaining
}

// extractJSON extracts a JSON object from the start of a string.
// Returns the JSON string, the remaining content, and success status.
func extractJSON(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '{' {
		return "", s, false
	}

	// Count braces to find the end of the JSON object
	depth := 0
	inString := false
	escaped := false

	for i, c := range s {
		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				// Found complete JSON object
				return s[:i+1], s[i+1:], true
			}
		}
	}

	// Incomplete JSON (truncated)
	// Return what we have, it might still be useful
	return s, "", false
}

// ConvertMistralResponse processes a response that may contain Mistral tool calls.
// If Mistral format is detected, it parses the tool calls and returns them.
// Otherwise, returns the content as-is with no tool calls.
func ConvertMistralResponse(content string, existingToolCalls []ToolCall) (string, []ToolCall) {
	// If we already have OpenAI-format tool calls, use those
	if len(existingToolCalls) > 0 {
		return content, existingToolCalls
	}

	// Check for Mistral format
	if !IsMistralToolCallFormat(content) {
		return content, nil
	}

	// Parse Mistral format
	toolCalls, remaining := ParseMistralToolCalls(content)
	return remaining, toolCalls
}

// ExtractCodeFromMistralResponse handles responses where Devstral may have
// wrapped code in tool calls or other formats. This extracts just the code content.
// It handles both complete and truncated JSON (common when max_tokens is hit).
func ExtractCodeFromMistralResponse(content string) string {
	// If it's a tool call, extract the content argument
	if !IsMistralToolCallFormat(content) {
		return content
	}

	// Try parsing normally first
	toolCalls, _ := ParseMistralToolCalls(content)
	for _, tc := range toolCalls {
		if tc.Function.Name == "write_file" {
			extracted := extractContentFromArgs(tc.Function.Arguments)
			if extracted != "" {
				return extracted
			}
		}
	}

	// If normal parsing failed, try direct extraction from raw content
	// This handles truncated JSON where the tool call wasn't fully parsed
	extracted := extractContentFromArgs(content)
	if extracted != "" {
		return extracted
	}

	return content
}

// extractContentFromArgs extracts the "content" field from JSON arguments.
// Handles both complete and truncated JSON.
func extractContentFromArgs(args string) string {
	// Look for "content": "..." or "content": '...'
	contentStart := strings.Index(args, `"content"`)
	if contentStart == -1 {
		return ""
	}

	// Find the value after "content":
	colonIdx := strings.Index(args[contentStart:], `:`)
	if colonIdx == -1 {
		return ""
	}
	valueStart := contentStart + colonIdx + 1

	// Skip whitespace
	for valueStart < len(args) && (args[valueStart] == ' ' || args[valueStart] == '\n' || args[valueStart] == '\t') {
		valueStart++
	}

	if valueStart >= len(args) {
		return ""
	}

	// Check for string start
	if args[valueStart] != '"' {
		return ""
	}

	// Extract string value (handle escapes)
	end := valueStart + 1
	for end < len(args) {
		if args[end] == '\\' && end+1 < len(args) {
			end += 2
			continue
		}
		if args[end] == '"' {
			// Found complete string end
			return unescapeJSON(args[valueStart+1 : end])
		}
		end++
	}

	// String was truncated - return what we have
	// This is better than returning nothing for truncated responses
	if end > valueStart+1 {
		return unescapeJSON(args[valueStart+1 : end])
	}

	return ""
}

// unescapeJSON handles common JSON escape sequences.
func unescapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\r`, "\r")
	return s
}
