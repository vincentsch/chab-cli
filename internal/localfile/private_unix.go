//go:build !windows

package localfile

func privateOutputSupported() error {
	return nil
}
