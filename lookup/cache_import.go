package lookup

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dealer-lookup/models"
)

func WarmCacheFromDetails(c *Cache, paths string) (int, error) {
	imported := 0
	for _, pattern := range splitPathList(paths) {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return imported, err
		}
		if len(matches) == 0 {
			matches = []string{pattern}
		}
		for _, path := range matches {
			n, err := warmCacheFromDetailsFile(c, path)
			if err != nil {
				return imported, err
			}
			imported += n
		}
	}
	return imported, nil
}

func splitPathList(paths string) []string {
	parts := strings.FieldsFunc(paths, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func warmCacheFromDetailsFile(c *Cache, path string) (int, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("read warmup details %s: %w", path, err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	header := headerIndex(rows[0])
	imported := 0
	for _, row := range rows[1:] {
		v := models.Vehicle{
			VIN:   field(row, header, "vin"),
			Year:  atoi(field(row, header, "year")),
			Make:  field(row, header, "make"),
			Model: field(row, header, "model"),
			Trim:  field(row, header, "trim"),
			Price: firstInt(row, header, "price", "current_price"),
			Miles: firstInt(row, header, "miles"),
		}
		res := models.LookupResult{
			VIN:                v.VIN,
			SuggestedPriceLow:  firstInt(row, header, "suggested_price_low"),
			SuggestedPriceHigh: firstInt(row, header, "suggested_price_high"),
			RecommendedPrice:   firstInt(row, header, "recommended_price"),
			Confidence:         firstInt(row, header, "confidence", "lookup_confidence"),
			Summary:            field(row, header, "summary", "dealer_summary"),
			Flags:              splitFlags(field(row, header, "flags", "lookup_flags")),
			Source:             field(row, header, "source", "lookup_source"),
		}
		if v.VIN == "" || v.Year == 0 || v.Make == "" || v.Model == "" || v.Price == 0 || res.RecommendedPrice == 0 {
			continue
		}
		if res.Source == "" {
			res.Source = "cache-warmup"
		}
		if res.Confidence == 0 {
			res.Confidence = 65
		}
		c.Put(v, res)
		imported++
	}
	return imported, nil
}

func headerIndex(header []string) map[string]int {
	out := map[string]int{}
	for i, h := range header {
		out[normalizeHeader(h)] = i
	}
	return out
}

func normalizeHeader(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func field(row []string, header map[string]int, names ...string) string {
	for _, name := range names {
		idx, ok := header[normalizeHeader(name)]
		if ok && idx >= 0 && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
	}
	return ""
}

func firstInt(row []string, header map[string]int, names ...string) int {
	for _, name := range names {
		if n := atoi(field(row, header, name)); n != 0 {
			return n
		}
	}
	return 0
}

func atoi(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

func splitFlags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.FieldsFunc(s, func(r rune) bool {
		return r == '|' || r == ';'
	})
	out := []string{}
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
