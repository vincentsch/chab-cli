// Package doctor implements the local readiness report command.
//
// Doctor is intentionally diagnostic: local setup problems can become failing
// findings, while live API/key problems are rendered as warnings and do not use
// the normal API exit-code categories.
package doctor
