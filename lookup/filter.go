package lookup

import (
	"dealer-lookup/config"
	"dealer-lookup/models"
)

func ShouldLookup(v models.Vehicle, cfg config.Config, cache Cache) (bool, string, models.LookupResult) {
	if v.VIN == "" {
		return false, "missing VIN", models.LookupResult{}
	}
	if res, ok := cache.GetFresh(v, cfg.CacheTTL); ok {
		return false, "cached", res
	}
	return true, "inventory lookup needed", models.LookupResult{}
}
