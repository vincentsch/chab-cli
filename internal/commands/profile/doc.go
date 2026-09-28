// Package profile implements the local profile and non-secret config command
// families. Every command is local-only: it never calls the API, never builds
// an API client, and never prints stored API keys.
package profile
