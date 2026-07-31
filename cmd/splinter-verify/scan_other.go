//go:build !linux

package main

import (
	"errors"
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/verify"
)

// scan is unavailable off Linux (HCI sockets are Linux-only). The stub keeps the
// tree building and the pure analysis testable on the dev Mac.
func scan(index int, window time.Duration) ([]verify.Observation, error) {
	return nil, errors.New("splinter-verify scanning requires Linux")
}
