//go:build linux

package main

import (
	"time"

	"github.com/zachsis/splinter-uconnectcm4/internal/hci"
	"github.com/zachsis/splinter-uconnectcm4/internal/verify"
)

// scan takes exclusive control of hci<index>, passively scans for the window,
// and maps the observed advertising reports to verify.Observations. It reuses
// the daemon's HCI transport, so it shares the same controller bring-up.
func scan(index int, window time.Duration) ([]verify.Observation, error) {
	conn, err := hci.New(index)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	reports, err := conn.Scan(window)
	if err != nil {
		return nil, err
	}

	obs := make([]verify.Observation, 0, len(reports))
	for _, r := range reports {
		id, hasMfg, name, fastPair, appleFindMy := verify.ParseAdvData(r.Data)
		obs = append(obs, verify.Observation{
			MAC:         r.Addr,
			Connectable: r.Connectable,
			CompanyID:   id,
			HasMfg:      hasMfg,
			Name:        name,
			FastPair:    fastPair,
			AppleFindMy: appleFindMy,
		})
	}
	return obs, nil
}
