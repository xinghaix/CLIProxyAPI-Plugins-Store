package store

import (
	"context"
	"testing"
)

func TestPricesIncludesUpdatedAt(t *testing.T) {
	store, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.ReplacePrices(context.Background(), map[string]Price{
		"gpt-test": {Prompt: 1, Completion: 2, Source: "manual"},
	}); err != nil {
		t.Fatal(err)
	}
	prices, err := store.Prices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	price, ok := prices["gpt-test"]
	if !ok || price.UpdatedAtMS <= 0 {
		t.Fatalf("prices=%#v", prices)
	}
}
