//go:build !unix

package files

import "os"

func openReadonly(path string) (*os.File, error) {
	return os.Open(path)
}
