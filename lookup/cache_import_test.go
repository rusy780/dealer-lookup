package lookup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dealer-lookup/models"
)

func TestWarmCacheFromDetailsImportsVehicleDetails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vehicle_details.csv")
	content := `VIN,Year,Make,Model,Trim,Price,Miles,lookup_group,suggested_price_low,suggested_price_high,recommended_price,confidence,summary,flags,source,cached
1C4PJMDX0PD000001,2024,Jeep,Grand Cherokee,Limited,32000,41234,"",29000,31000,30000,75,Good retail unit,low miles,test,false
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cache := Cache{}
	imported, err := WarmCacheFromDetails(&cache, path)
	if err != nil {
		t.Fatalf("WarmCacheFromDetails() error = %v", err)
	}
	if imported != 1 {
		t.Fatalf("imported = %d, want 1", imported)
	}

	v := models.Vehicle{
		VIN:   "1C4PJMDX0PD000001",
		Year:  2024,
		Make:  "Jeep",
		Model: "Grand Cherokee",
		Trim:  "Limited",
		Price: 32000,
		Miles: 41234,
	}
	got, ok := cache.GetFresh(v, time.Hour)
	if !ok {
		t.Fatal("warm cache entry was not fresh")
	}
	if got.RecommendedPrice != 30000 || got.Source != "test" {
		t.Fatalf("cached result = %+v", got)
	}
}
