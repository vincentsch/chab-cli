package rawapi

import (
	"net/http"
	"net/url"
	"strings"
)

type normalizedSegment struct {
	escaped string
	decoded string
}

func isOneTimeSecretRoute(method, requestPath string) bool {
	segments, ok := normalizedSegments(requestPath)
	if !ok {
		return false
	}
	if method == http.MethodGet {
		return isOneTimeManagementApprovalRoute(segments)
	}
	if method != http.MethodPost {
		return false
	}
	switch len(segments) {
	case 1:
		return fixedSegment(segments[0], "tokens")
	case 2:
		return fixedSegment(segments[0], "webhooks") &&
			fixedSegment(segments[1], "endpoints")
	case 4:
		return fixedSegment(segments[0], "webhooks") &&
			fixedSegment(segments[1], "endpoints") &&
			opaqueRouteID(segments[2].decoded) &&
			fixedSegment(segments[3], "rotate-secret")
	default:
		return false
	}
}

func isOneTimeManagementApprovalRoute(segments []normalizedSegment) bool {
	return len(segments) == 2 &&
		fixedSegment(segments[0], "management-approvals") &&
		opaqueRouteID(segments[1].decoded)
}

func normalizedSegments(requestPath string) ([]normalizedSegment, bool) {
	if requestPath == "" || strings.ContainsAny(requestPath, "?#") {
		return nil, false
	}
	if strings.HasPrefix(requestPath, "/") {
		requestPath = strings.TrimPrefix(requestPath, "/")
	}
	if requestPath == "" || strings.HasPrefix(requestPath, "/") ||
		strings.HasSuffix(requestPath, "/") ||
		strings.Contains(requestPath, "//") {
		return nil, false
	}
	parts := strings.Split(requestPath, "/")
	segments := make([]normalizedSegment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return nil, false
		}
		segments = append(segments, normalizedSegment{escaped: part, decoded: decoded})
	}
	return segments, true
}

func fixedSegment(segment normalizedSegment, literal string) bool {
	return segment.escaped == literal && segment.decoded == literal
}

func opaqueRouteID(segment string) bool {
	if segment == "" || pathSpecialAfterDecoding(segment) {
		return false
	}
	return true
}

func pathSpecialAfterDecoding(segment string) bool {
	for {
		if segment == "." || segment == ".." ||
			strings.Contains(segment, "/") {
			return true
		}
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == segment {
			return false
		}
		segment = decoded
	}
}
