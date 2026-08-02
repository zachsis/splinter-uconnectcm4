//go:build linux

package main

import (
	"time"

	"github.com/zachsis/helmofhades/internal/hci"
	"github.com/zachsis/helmofhades/internal/verify"
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
		info := verify.ParseAdvData(r.Data)
		obs = append(obs, verify.Observation{
			MAC:                  r.Addr,
			Connectable:          r.Connectable,
			CompanyID:            info.CompanyID,
			HasMfg:               info.HasMfg,
			Name:                 info.Name,
			FastPair:             info.FastPair,
			FastPairDiscoverable: info.FastPairDiscoverable,
			AppleFindMy:          info.AppleFindMy,
			Tile:                 info.Tile,
		})
	}
	return obs, nil
}
