//go:build windows

package localfile

import "fmt"

func privateOutputSupported() error {
	return fmt.Errorf("private output cannot guarantee owner-only ACLs on Windows")
}
