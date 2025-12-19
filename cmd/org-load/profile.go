package main

import (
	"errors"
	"time"
)

type profile struct {
	Name           string
	VUs            int
	Duration       time.Duration
	Targets        []target
	DefaultP99MS   int
	EffectiveDate  string
	RequiresWrites bool
}

type target struct {
	Endpoint string
	Method   string
	Path     string
	Weight   int
	Body     func(*runOptions) ([]byte, error)
}

func builtinProfile(name string, effectiveDate string) (profile, error) {
	switch name {
	case "org_read_1k":
		return profile{
			Name:          name,
			VUs:           20,
			Duration:      120 * time.Second,
			DefaultP99MS:  200,
			EffectiveDate: effectiveDate,
			Targets: []target{
				{
					Endpoint: "hierarchies.get",
					Method:   "GET",
					Path:     "/org/api/hierarchies?type=OrgUnit&effective_date=" + effectiveDate,
					Weight:   10,
				},
			},
		}, nil
	case "org_read_10k":
		return profile{
			Name:          name,
			VUs:           40,
			Duration:      180 * time.Second,
			DefaultP99MS:  500,
			EffectiveDate: effectiveDate,
			Targets: []target{
				{
					Endpoint: "hierarchies.get",
					Method:   "GET",
					Path:     "/org/api/hierarchies?type=OrgUnit&effective_date=" + effectiveDate,
					Weight:   10,
				},
			},
		}, nil
	case "org_mix_read_write":
		return profile{
			Name:           name,
			VUs:            20,
			Duration:       120 * time.Second,
			DefaultP99MS:   500,
			EffectiveDate:  effectiveDate,
			RequiresWrites: true,
			Targets: []target{
				{
					Endpoint: "hierarchies.get",
					Method:   "GET",
					Path:     "/org/api/hierarchies?type=OrgUnit&effective_date=" + effectiveDate,
					Weight:   8,
				},
				{
					Endpoint: "batch.post",
					Method:   "POST",
					Path:     "/org/api/batch",
					Weight:   2,
					Body:     buildBatchDryRunCreateNode,
				},
			},
		}, nil
	default:
		return profile{}, errors.New("unknown profile")
	}
}
