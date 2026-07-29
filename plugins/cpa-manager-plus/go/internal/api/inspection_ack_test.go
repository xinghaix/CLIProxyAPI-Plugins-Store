package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestInspectionAcknowledgeRouteOnlyMarksManualResults(t *testing.T) {
	runtime, err := app.New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	ctx := context.Background()
	run, err := runtime.Store().StartInspectionRun(ctx, "manual", "", "{}")
	if err != nil {
		t.Fatal(err)
	}
	review, err := runtime.Store().InsertInspectionResult(ctx, store.InspectionResult{RunID: run.ID, AccountKey: "review", FileName: "review.json", Action: "review", ActionStatus: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	disable, err := runtime.Store().InsertInspectionResult(ctx, store.InspectionResult{RunID: run.ID, AccountKey: "disable", FileName: "disable.json", Action: "disable", ActionStatus: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Store().FinishInspectionRun(ctx, run, "completed", ""); err != nil {
		t.Fatal(err)
	}

	request := fmt.Sprintf(`{"method":"POST","path":"/v0/management/codex-inspection/runs/%d/acknowledge","body":{"resultIds":[%d,%d]}}`, run.ID, review.ID, disable.ID)
	response := Handle(ctx, runtime, []byte(request))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("acknowledge = %d: %s", response.StatusCode, response.Body)
	}
	var payload struct {
		Outcomes []struct {
			ID      int64 `json:"id"`
			Success bool  `json:"success"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Outcomes) != 2 || !payload.Outcomes[0].Success || payload.Outcomes[1].Success {
		t.Fatalf("acknowledge outcomes = %#v", payload.Outcomes)
	}

	results, err := runtime.Store().InspectionResults(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].ActionStatus != "acknowledged" || results[0].ExecutedAction != "acknowledge" {
		t.Fatalf("review result = %#v", results[0])
	}
	if results[1].ActionStatus != "pending" {
		t.Fatalf("server action was changed = %#v", results[1])
	}
}
