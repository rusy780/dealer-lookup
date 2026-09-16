package exportcsv

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dealer-lookup/models"
)

var AddedHeaders = []string{
	"lookup_group",
	"lookup_needed",
	"lookup_used",
	"lookup_source",
	"lookup_cached",
	"suggested_price_low",
	"suggested_price_high",
	"recommended_price",
	"lookup_confidence",
	"dealer_summary",
	"lookup_flags",
}

var BaseHeaders = []string{"VIN", "Year", "Make", "Model", "Trim", "Price", "Miles"}

func WriteEnriched(path string, vehicles []models.Vehicle, results map[string]models.LookupResult, lookupNeeded map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	outHeaders := append([]string{}, BaseHeaders...)
	outHeaders = append(outHeaders, AddedHeaders...)
	if err := w.Write(outHeaders); err != nil {
		return err
	}

	for _, v := range vehicles {
		row := []string{
			v.VIN,
			strconv.Itoa(v.Year),
			v.Make,
			v.Model,
			v.Trim,
			strconv.Itoa(v.Price),
			strconv.Itoa(v.Miles),
		}
		res, has := results[v.VIN]
		row = append(row,
			v.GroupKey(),
			boolStr(lookupNeeded[v.VIN]),
			boolStr(has),
			res.Source,
			boolStr(res.Cached),
			itoaZeroBlank(res.SuggestedPriceLow),
			itoaZeroBlank(res.SuggestedPriceHigh),
			itoaZeroBlank(res.RecommendedPrice),
			itoaZeroBlank(res.Confidence),
			res.Summary,
			strings.Join(res.Flags, "|"),
		)
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func WriteDetails(path string, vehicles []models.Vehicle, results map[string]models.LookupResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{
		"VIN", "Year", "Make", "Model", "Trim", "Price", "Miles",
		"lookup_group", "suggested_price_low", "suggested_price_high", "recommended_price", "confidence", "summary", "flags", "source", "cached",
	})
	for _, v := range vehicles {
		res, ok := results[v.VIN]
		if !ok {
			continue
		}
		w.Write([]string{
			v.VIN,
			strconv.Itoa(v.Year),
			v.Make,
			v.Model,
			v.Trim,
			strconv.Itoa(v.Price),
			strconv.Itoa(v.Miles),
			v.GroupKey(),
			strconv.Itoa(res.SuggestedPriceLow),
			strconv.Itoa(res.SuggestedPriceHigh),
			strconv.Itoa(res.RecommendedPrice),
			strconv.Itoa(res.Confidence),
			res.Summary,
			strings.Join(res.Flags, "|"),
			res.Source,
			boolStr(res.Cached),
		})
	}
	return w.Error()
}

type TokenEstimate struct {
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	EstimatedCost      float64
	EstimatedCost1M    float64
	RowsEstimated      int
	PricePerMillionTok float64
}

func WriteManifest(path string, total, apiCalls, cached int, tokenEstimate TokenEstimate, enrichedPath, detailsPath, originalPath string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf(`{
  "total_rows": %d,
  "new_lookup_calls": %d,
  "cached_results_used": %d,
  "estimated_prompt_tokens": %d,
  "estimated_completion_tokens": %d,
  "estimated_total_tokens": %d,
  "token_price_per_million": %.6f,
  "estimated_lookup_cost": %.6f,
  "estimated_cost_per_1m_rows": %.6f,
  "original_copy": %q,
  "enriched_csv": %q,
  "vehicle_details_csv": %q
}
`, total, apiCalls, cached, tokenEstimate.PromptTokens, tokenEstimate.CompletionTokens, tokenEstimate.TotalTokens, tokenEstimate.PricePerMillionTok, tokenEstimate.EstimatedCost, tokenEstimate.EstimatedCost1M, originalPath, enrichedPath, detailsPath)
	return os.WriteFile(path, []byte(content), 0644)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
func itoaZeroBlank(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
