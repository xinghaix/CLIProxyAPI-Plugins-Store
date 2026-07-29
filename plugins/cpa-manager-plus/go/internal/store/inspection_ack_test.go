package store

import (
	"context"
	"testing"
)

func TestAcknowledgeInspectionResultsOnlyMarksManualActions(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	run, err := database.StartInspectionRun(ctx, "manual", "", "{}")
	if err != nil {
		t.Fatal(err)
	}
	results := []InspectionResult{
		{RunID: run.ID, AccountKey: "review", FileName: "review.json", Action: "review", ActionStatus: "pending", ActionError: "upstream error"},
		{RunID: run.ID, AccountKey: "reauth", FileName: "reauth.json", Action: "reauth", ActionStatus: "failed", ActionError: "invalid token"},
		{RunID: run.ID, AccountKey: "disable", FileName: "disable.json", Action: "disable", ActionStatus: "pending"},
		{RunID: run.ID, AccountKey: "keep", FileName: "keep.json", Action: "keep", ActionStatus: "pending"},
	}
	for index, result := range results {
		stored, err := database.InsertInspectionResult(ctx, result)
		if err != nil {
			t.Fatal(err)
		}
		results[index] = stored
	}
	if _, err := database.FinishInspectionRun(ctx, run, "completed", ""); err != nil {
		t.Fatal(err)
	}

	response, err := database.AcknowledgeInspectionResults(ctx, run.ID, []int64{results[0].ID, results[1].ID, results[2].ID, results[3].ID})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := response["outcomes"].([]map[string]any)
	byID := map[int64]map[string]any{}
	for _, outcome := range outcomes {
		byID[outcome["id"].(int64)] = outcome
	}
	if byID[results[0].ID]["success"] != true || byID[results[1].ID]["success"] != true {
		t.Fatalf("manual acknowledgement outcomes = %#v", outcomes)
	}
	if byID[results[2].ID]["success"] != false || byID[results[3].ID]["success"] != false {
		t.Fatalf("non-manual acknowledgement outcomes = %#v", outcomes)
	}

	stored, err := database.InspectionResults(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	resultByID := map[int64]InspectionResult{}
	for _, result := range stored {
		resultByID[result.ID] = result
	}
	for _, id := range []int64{results[0].ID, results[1].ID} {
		result := resultByID[id]
		if result.ActionStatus != "acknowledged" || result.ExecutedAction != "acknowledge" || result.ActionError != "" {
			t.Fatalf("acknowledged result = %#v", result)
		}
	}
	if resultByID[results[2].ID].ActionStatus != "pending" || resultByID[results[3].ID].ActionStatus != "pending" {
		t.Fatalf("non-manual results changed = %#v", resultByID)
	}

	retry, err := database.AcknowledgeInspectionResults(ctx, run.ID, []int64{results[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if retry["outcomes"].([]map[string]any)[0]["success"] != false {
		t.Fatalf("repeat acknowledgement = %#v", retry)
	}
}

func TestAcknowledgeInspectionResultsRequiresCompletedRun(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	run, err := database.StartInspectionRun(ctx, "manual", "", "{}")
	if err != nil {
		t.Fatal(err)
	}
	result, err := database.InsertInspectionResult(ctx, InspectionResult{RunID: run.ID, AccountKey: "review", FileName: "review.json", Action: "review", ActionStatus: "pending"})
	if err != nil {
		t.Fatal(err)
	}

	response, err := database.AcknowledgeInspectionResults(ctx, run.ID, []int64{result.ID})
	if err != nil {
		t.Fatal(err)
	}
	if response["outcomes"].([]map[string]any)[0]["success"] != false {
		t.Fatalf("running run acknowledgement = %#v", response)
	}
	stored, err := database.InspectionResults(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].ActionStatus != "pending" {
		t.Fatalf("running run result changed = %#v", stored[0])
	}
}
