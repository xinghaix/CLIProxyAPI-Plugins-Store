package store

import (
	"context"
	"strings"
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

func TestDeletePriceRemovesOnlyRequestedModel(t *testing.T) {
	store, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.ReplacePrices(ctx, map[string]Price{
		"gpt-test":    {Prompt: 1, Completion: 2},
		"acme/claude": {Prompt: 3, Completion: 4},
	}); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.DeletePrice(ctx, "gpt-test")
	if err != nil || !deleted {
		t.Fatalf("DeletePrice(gpt-test) = %v, %v", deleted, err)
	}
	prices, err := store.Prices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prices["gpt-test"]; ok {
		t.Fatalf("deleted model remained: %#v", prices)
	}
	if _, ok := prices["acme/claude"]; !ok {
		t.Fatalf("unrelated model was removed: %#v", prices)
	}

	deleted, err = store.DeletePrice(ctx, "gpt-test")
	if err != nil || deleted {
		t.Fatalf("second DeletePrice(gpt-test) = %v, %v", deleted, err)
	}
	if deleted, err = store.DeletePrice(ctx, "acme/claude"); err != nil || !deleted {
		t.Fatalf("DeletePrice(acme/claude) = %v, %v", deleted, err)
	}
	if _, err := store.DeletePrice(ctx, ""); err == nil {
		t.Fatal("empty model must be rejected")
	}
	if _, err := store.DeletePrice(ctx, strings.Repeat("x", 257)); err == nil {
		t.Fatal("overlong model must be rejected")
	}
}
