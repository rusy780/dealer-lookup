package lookup

import (
	"testing"
	"time"

	"dealer-lookup/config"
	"dealer-lookup/models"
)

func TestShouldLookupUsesFreshCacheToAvoidNewLookup(t *testing.T) {
	cfg := config.Config{CacheTTL: 24 * time.Hour}
	v := testVehicle()
	cache := Cache{}
	cache.Put(v, models.LookupResult{VIN: v.VIN, RecommendedPrice: 30000})

	need, reason, cached := ShouldLookup(v, cfg, cache)
	if need {
		t.Fatal("ShouldLookup() requested a lookup despite a fresh cache hit")
	}
	if reason != "cached" {
		t.Fatalf("reason = %q, want cached", reason)
	}
	if !cached.Cached {
		t.Fatal("cached result was not marked cached")
	}
}

func TestShouldLookupSkipsVehiclesWithoutVIN(t *testing.T) {
	cfg := config.Config{CacheTTL: 24 * time.Hour}

	missingVIN := testVehicle()
	missingVIN.VIN = ""
	need, reason, _ := ShouldLookup(missingVIN, cfg, Cache{})
	if need || reason != "missing VIN" {
		t.Fatalf("missing VIN vehicle: need=%v reason=%q", need, reason)
	}
}

func TestShouldLookupRequestsLookupWhenNotCached(t *testing.T) {
	cfg := config.Config{CacheTTL: 24 * time.Hour}
	v := testVehicle()

	need, reason, cached := ShouldLookup(v, cfg, Cache{})
	if !need {
		t.Fatal("ShouldLookup() did not request lookup for uncached vehicle")
	}
	if reason != "inventory lookup needed" {
		t.Fatalf("reason = %q, want inventory lookup needed", reason)
	}
	if cached.VIN != "" || cached.RecommendedPrice != 0 || cached.SuggestedPriceLow != 0 {
		t.Fatalf("cached = %+v, want zero value", cached)
	}
}
