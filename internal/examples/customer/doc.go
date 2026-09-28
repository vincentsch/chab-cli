// Package customer is a compiled-in teaching artifact for the SaaS CLI
// authoring guide (docs/saas-cli-authoring.md), not Chab-SaaS product
// behavior. It demonstrates the feature-module pattern end to end: production
// code never imports it, cmd/chab never references it, and the production
// module list never includes it, so it exists only in command trees that
// tests construct explicitly.
package customer
