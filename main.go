package main

import (
	"context"
	"fmt"
	"log"

	"dealer-lookup/config"
	"dealer-lookup/exportcsv"
	"dealer-lookup/importcsv"
	"dealer-lookup/lookup"
	"dealer-lookup/models"
	"dealer-lookup/storage"
)

func main() {
	cfg := config.Load()

	vehicles, err := importcsv.Read(cfg.InputPath)
	must(err)
	paths, err := storage.PrepareRun(cfg.OutputDir, cfg.InputPath)
	must(err)

	cache, err := lookup.LoadCache(cfg.CachePath)
	must(err)
	warmedCount, err := lookup.WarmCacheFromDetails(&cache, cfg.CacheWarmupPaths)
	must(err)

	results := map[string]models.LookupResult{}
	lookupNeeded := map[string]bool{}
	pendingLookup := []models.Vehicle{}
	cachedCount := 0

	for _, v := range vehicles {
		need, reason, cached := lookup.ShouldLookup(v, cfg, cache)
		_ = reason
		if cached.VIN != "" || cached.RecommendedPrice != 0 || cached.SuggestedPriceLow != 0 {
			results[v.VIN] = cached
			cachedCount++
		}
		if need {
			lookupNeeded[v.VIN] = true
			pendingLookup = append(pendingLookup, v)
		}
	}

	groups := lookup.GroupVehiclesForLookup(pendingLookup)
	selectedGroups := groups
	if cfg.MaxLookupsPerRun > 0 && len(selectedGroups) > cfg.MaxLookupsPerRun {
		selectedGroups = selectedGroups[:cfg.MaxLookupsPerRun]
	}
	toLookup := make([]models.Vehicle, 0, len(selectedGroups))
	for _, group := range selectedGroups {
		toLookup = append(toLookup, group.Representative)
	}

	tokenEstimate := lookup.EstimateTokenUse(toLookup, cfg.BatchSize, cfg.TokenPrice)
	if len(pendingLookup) > 0 {
		tokenEstimate.RowsEstimated = len(pendingLookup)
		tokenEstimate.EstimatedCost1M = tokenEstimate.EstimatedCost / float64(len(pendingLookup)) * 1_000_000
	}
	fmt.Println("loaded rows:", len(vehicles))
	fmt.Println("uncached rows:", len(pendingLookup), "groups:", len(groups), "queued openclaw lookups:", len(toLookup), "cached:", cachedCount, "warmup imported:", warmedCount, "batch_size:", cfg.BatchSize, "concurrency:", cfg.Concurrency)
	fmt.Printf("estimated tokens: %d cost: $%.4f projected per 1M rows: $%.2f\n", tokenEstimate.TotalTokens, tokenEstimate.EstimatedCost, tokenEstimate.EstimatedCost1M)
	if len(toLookup) > 0 {
		fmt.Println("starting lookups; progress is written once per batch")
	}

	client := lookup.NewClient(cfg)
	newResults, err := client.BatchLookup(context.Background(), toLookup)
	must(err)

	groupedResultCount := 0
	for _, group := range selectedGroups {
		res, ok := newResults[group.Representative.VIN]
		if !ok {
			continue
		}
		for _, v := range group.Members {
			memberRes := res
			memberRes.VIN = v.VIN
			memberRes.Cached = false
			if memberRes.Source == "" {
				memberRes.Source = "openclaw"
			}
			results[v.VIN] = memberRes
			cache.Put(v, memberRes)
			groupedResultCount++
		}
	}

	must(cache.Save(cfg.CachePath))
	must(exportcsv.WriteEnriched(paths.Enriched, vehicles, results, lookupNeeded))
	must(exportcsv.WriteDetails(paths.Details, vehicles, results))
	must(exportcsv.WriteManifest(paths.Manifest, len(vehicles), len(toLookup), cachedCount, exportcsv.TokenEstimate(tokenEstimate), paths.Enriched, paths.Details, paths.Original))

	fmt.Println("done")
	fmt.Println("run_id:", paths.RunID)
	fmt.Println("original:", paths.Original)
	fmt.Println("enriched:", paths.Enriched)
	fmt.Println("details:", paths.Details)
	fmt.Println("manifest:", paths.Manifest)
	fmt.Println("new openclaw lookups:", len(toLookup), "grouped rows filled:", groupedResultCount, "cached:", cachedCount)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
