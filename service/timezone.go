package service

import "time"

// The desktop and Android core exchange human-readable timestamps.  Pin the
// process-wide location so a device with a UTC default does not emit logs that
// are eight hours behind the user's local (China) time.
func init() {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		time.Local = loc
	}
}
