// Package internal holds build-time metadata such as the software version.
package internal

import (
	"fmt"
	"strconv"
	"time"
)

var (
	commitVersion = "0.1.0"      // Should be updated during build (see Makefile LDFLAGS)
	commitDate    = "1780586040" // commitDate in Epoch seconds (updated during build)
)

// GetVersion returns the version and, when available, the commit date.
func GetVersion() string {
	msg := commitVersion
	if commitDate != "" {
		if seconds, err := strconv.Atoi(commitDate); err == nil {
			t := time.Unix(int64(seconds), 0)
			msg += fmt.Sprintf(", date: %s", t.Format("2006-01-02"))
		}
	}
	return msg
}
