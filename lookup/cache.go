package lookup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dealer-lookup/models"
)

type Cache struct {
	Items map[string]models.LookupResult `json:"items"`
}

type VehicleGroup struct {
	Key            string
	Representative models.Vehicle
	Members        []models.Vehicle
}

func LoadCache(path string) (Cache, error) {
	c := Cache{Items: map[string]models.LookupResult{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if len(b) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Items == nil {
		c.Items = map[string]models.LookupResult{}
	}
	return c, nil
}

func (c Cache) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func (c Cache) GetFresh(v models.Vehicle, ttl time.Duration) (models.LookupResult, bool) {
	key := ExactKey(v)
	res, ok := c.Items[key]
	if ok && time.Since(res.CreatedAt) <= ttl {
		res.Cached = true
		return res, true
	}
	// Group-level reuse saves money across similar cars.
	gkey := GroupKey(v)
	res, ok = c.Items[gkey]
	if ok && time.Since(res.CreatedAt) <= ttl {
		res.Cached = true
		res.VIN = v.VIN
		return res, true
	}
	return models.LookupResult{}, false
}

func (c *Cache) Put(v models.Vehicle, res models.LookupResult) {
	if c.Items == nil {
		c.Items = map[string]models.LookupResult{}
	}
	res.CreatedAt = time.Now().UTC()
	res.InputHash = InputHash(v)
	c.Items[ExactKey(v)] = res
	// Also store a group cache result so the next similar vehicle can reuse it.
	groupRes := res
	groupRes.VIN = ""
	c.Items[GroupKey(v)] = groupRes
}

func ExactKey(v models.Vehicle) string {
	return fmt.Sprintf("vin:%s|price:%d|miles:%d", strings.ToUpper(strings.TrimSpace(v.VIN)), v.Price, v.Miles)
}

func GroupKey(v models.Vehicle) string {
	return v.GroupKey()
}

func InputHash(v models.Vehicle) string {
	s := fmt.Sprintf("%s|%d|%d|%d|%s|%s|%s", v.VIN, v.Year, v.Price, v.Miles, v.Make, v.Model, v.Trim)
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func GroupVehiclesForLookup(vehicles []models.Vehicle) []VehicleGroup {
	groups := []VehicleGroup{}
	byKey := map[string]int{}
	for _, v := range vehicles {
		key := GroupKey(v)
		idx, ok := byKey[key]
		if !ok {
			byKey[key] = len(groups)
			groups = append(groups, VehicleGroup{
				Key:            key,
				Representative: v,
				Members:        []models.Vehicle{v},
			})
			continue
		}
		groups[idx].Members = append(groups[idx].Members, v)
	}
	return groups
}
