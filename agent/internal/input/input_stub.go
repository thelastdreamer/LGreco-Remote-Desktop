//go:build !windows

package input

import "log"

func Apply(ev Event) error {
	log.Printf("input ignored on this OS: %+v", ev)
	return nil
}
