//go:build !linux

package files

import "os"

func changeTime(os.FileInfo) (int64, bool) { return 0, false }
