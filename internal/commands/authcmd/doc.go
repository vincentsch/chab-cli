// Package authcmd implements the authentication and identity commands.
//
// The package owns guided setup, the functional auth family, the top-level
// login/logout aliases, and whoami. It receives all process state through
// cmdutil.Factory so command code stays testable and does not read files,
// environment variables, or terminal state directly except through injected
// helpers.
package authcmd
