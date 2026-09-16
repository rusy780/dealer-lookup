package lookup

import (
	"path/filepath"
	"testing"
	"time"

	"dealer-lookup/models"
)

func TestCacheSaveLoadAndExactFreshHit(t *testing.T) {
	v := testVehicle()
	cache := Cache{}
	cache.Put(v, models.LookupResult{
		VIN:                v.VIN,
		SuggestedPriceLow:  29000,
		SuggestedPriceHigh: 31000,
		RecommendedPrice:   30000,
		Source:             "test",
	})

	path := filepath.Join(t.TempDir(), "nested", "cache.json")
	if err := cache.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := LoadCache(path)
	if err != nil {
		t.Fatalf("LoadCache() error = %v", err)
	}

	got, ok := loaded.GetFresh(v, 24*time.Hour)
	if !ok {
		t.Fatal("GetFresh() did not return an exact cache hit")
	}
	if !got.Cached {
		t.Fatal("GetFresh() did not mark exact result as cached")
	}
	if got.VIN != v.VIN {
		t.Fatalf("cached VIN = %q, want %q", got.VIN, v.VIN)
	}
	if got.RecommendedPrice != 30000 {
		t.Fatalf("recommended price = %d, want 30000", got.RecommendedPrice)
	}
	if got.InputHash != InputHash(v) {
		t.Fatalf("input hash = %q, want %q", got.InputHash, InputHash(v))
	}
}

func TestCacheGroupFreshHitReusesSimilarVehicle(t *testing.T) {
	original := testVehicle()
	similar := original
	similar.VIN = "2C4RC1BG0PR000002"
	similar.Price = original.Price + 500
	similar.Miles = original.Miles + 999

	cache := Cache{}
	cache.Put(original, models.LookupResult{
		VIN:                original.VIN,
		SuggestedPriceLow:  29000,
		SuggestedPriceHigh: 31000,
		RecommendedPrice:   30000,
		Source:             "test",
	})

	got, ok := cache.GetFresh(similar, time.Hour)
	if !ok {
		t.Fatal("GetFresh() did not return a group cache hit")
	}
	if !got.Cached {
		t.Fatal("GetFresh() did not mark group result as cached")
	}
	if got.VIN != similar.VIN {
		t.Fatalf("group cache VIN = %q, want %q", got.VIN, similar.VIN)
	}
	if got.RecommendedPrice != 30000 {
		t.Fatalf("recommended price = %d, want 30000", got.RecommendedPrice)
	}
}

func TestCacheGroupKeyIncludesPriceBucket(t *testing.T) {
	original := testVehicle()
	differentPriceBucket := original
	differentPriceBucket.VIN = "2C4RC1BG0PR000002"
	differentPriceBucket.Price = original.Price + models.PriceBucketSize

	cache := Cache{}
	cache.Put(original, models.LookupResult{
		VIN:              original.VIN,
		RecommendedPrice: 30000,
		Source:           "test",
	})

	if _, ok := cache.GetFresh(differentPriceBucket, time.Hour); ok {
		t.Fatal("GetFresh() returned a group cache hit across price buckets")
	}
}

func TestGroupVehiclesForLookupUsesYearMakeModelTrimPriceAndMileageBuckets(t *testing.T) {
	first := testVehicle()
	second := first
	second.VIN = "2C4RC1BG0PR000002"
	second.Price = first.Price + 500
	second.Miles = first.Miles + 999

	third := first
	third.VIN = "3C4RC1BG0PR000003"
	third.Price = first.Price + models.PriceBucketSize

	groups := GroupVehiclesForLookup([]models.Vehicle{first, second, third})
	if len(groups) != 2 {
		t.Fatalf("group count = %d, want 2", len(groups))
	}
	if len(groups[0].Members) != 2 {
		t.Fatalf("first group members = %d, want 2", len(groups[0].Members))
	}
	if groups[0].Representative.VIN != first.VIN {
		t.Fatalf("representative VIN = %q, want %q", groups[0].Representative.VIN, first.VIN)
	}
	if len(groups[1].Members) != 1 || groups[1].Members[0].VIN != third.VIN {
		t.Fatalf("second group = %+v, want third vehicle only", groups[1].Members)
	}
}

func TestCacheStaleEntriesAreIgnored(t *testing.T) {
	v := testVehicle()
	cache := Cache{Items: map[string]models.LookupResult{
		ExactKey(v): {
			VIN:              v.VIN,
			RecommendedPrice: 30000,
			CreatedAt:        time.Now().UTC().Add(-2 * time.Hour),
		},
	}}

	if _, ok := cache.GetFresh(v, time.Hour); ok {
		t.Fatal("GetFresh() returned a stale exact cache entry")
	}
}

func TestLoadCacheMissingAndEmptyFilesReturnUsableCache(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	cache, err := LoadCache(missing)
	if err != nil {
		t.Fatalf("LoadCache(missing) error = %v", err)
	}
	if cache.Items == nil {
		t.Fatal("LoadCache(missing) returned nil Items")
	}

	empty := filepath.Join(dir, "empty.json")
	if err := (Cache{}).Save(empty); err != nil {
		t.Fatalf("Save(empty) error = %v", err)
	}
	cache, err = LoadCache(empty)
	if err != nil {
		t.Fatalf("LoadCache(empty) error = %v", err)
	}
	if cache.Items == nil {
		t.Fatal("LoadCache(empty) returned nil Items")
	}
}

func testVehicle() models.Vehicle {
	return models.Vehicle{
		VIN:   "1C4PJMDX0PD000001",
		Year:  2024,
		Make:  "Jeep",
		Model: "Grand Cherokee",
		Trim:  "Limited",
		Price: 32000,
		Miles: 41234,
	}
}
