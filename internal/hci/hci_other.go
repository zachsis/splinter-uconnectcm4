//go:build !linux

package hci

import "errors"

// errUnsupported is returned by all transport operations off Linux. splinterd's
// HCI transport is Linux-only; this stub lets the tree build and unit-test the
// pure encoders on any OS (e.g. the dev Mac).
var errUnsupported = errors.New("splinterd HCI transport requires Linux")

// Conn is a non-Linux stub with the same method set as the Linux transport.
type Conn struct{}

func New(index int) (*Conn, error)                          { return nil, errUnsupported }
func (c *Conn) Close() error                                { return errUnsupported }
func (c *Conn) Reset() error                                { return errUnsupported }
func (c *Conn) SetAdvEnable(on bool) error                  { return errUnsupported }
func (c *Conn) SetRandomAddr(a [6]byte) error               { return errUnsupported }
func (c *Conn) SetAdvParams(minMs, maxMs uint16, t byte) error { return errUnsupported }
func (c *Conn) SetAdvData(ad []byte) error                  { return errUnsupported }
