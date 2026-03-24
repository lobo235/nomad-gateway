package nomad

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/nomad/api"
)

// newTestClient creates a Client pointed at the given test server URL.
// healthPollInterval is set to 10ms for fast tests.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	cfg := api.DefaultConfig()
	cfg.Address = serverURL

	c, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("creating nomad api client: %v", err)
	}

	return &Client{
		nomad:              c,
		log:                slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
		healthPollInterval: 10 * time.Millisecond,
	}
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/self" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"member": map[string]any{"Name": "test-node"}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.Ping(); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestPing_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.Ping(); err == nil {
		t.Error("expected Ping() to return error on 503, got nil")
	}
}

func TestListJobs(t *testing.T) {
	want := []*api.JobListStub{
		{ID: "minecraft-survival", Name: "minecraft-survival", Status: "running"},
		{ID: "minecraft-creative", Name: "minecraft-creative", Status: "stopped"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	jobs, err := c.ListJobs("")
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("ListJobs() returned %d jobs, want 2", len(jobs))
	}
	ids := map[string]bool{jobs[0].ID: true, jobs[1].ID: true}
	if !ids["minecraft-survival"] || !ids["minecraft-creative"] {
		t.Errorf("ListJobs() IDs = %v, want both minecraft-survival and minecraft-creative", ids)
	}
}

func TestListJobs_WithPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		if prefix != "minecraft" {
			t.Errorf("prefix = %q, want %q", prefix, "minecraft")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*api.JobListStub{{ID: "minecraft-survival", Name: "minecraft-survival"}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	jobs, err := c.ListJobs("minecraft")
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("ListJobs() returned %d jobs, want 1", len(jobs))
	}
}

func TestGetJob(t *testing.T) {
	jobID := "minecraft-survival"
	want := &api.Job{ID: &jobID, Status: strPtr("running")}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/"+jobID {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	job, err := c.GetJob(jobID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if *job.ID != jobID {
		t.Errorf("job.ID = %q, want %q", *job.ID, jobID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "job not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetJob("nonexistent")
	if err == nil {
		t.Error("GetJob() expected error for 404, got nil")
	}
}

func TestGetAllocInfo(t *testing.T) {
	allocID := "alloc-abc123"
	jobID := "minecraft-survival"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/allocation/"+allocID {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&api.Allocation{
			ID:    allocID,
			JobID: jobID,
			AllocatedResources: &api.AllocatedResources{
				Shared: api.AllocatedSharedResources{
					Ports: []api.PortMapping{
						{Label: "minecraft", Value: 25565, To: 25565, HostIP: "198.51.100.1"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	alloc, err := c.GetAllocInfo(allocID)
	if err != nil {
		t.Fatalf("GetAllocInfo() error = %v", err)
	}
	if alloc.ID != allocID {
		t.Errorf("alloc.ID = %q, want %q", alloc.ID, allocID)
	}
	if len(alloc.AllocatedResources.Shared.Ports) != 1 {
		t.Errorf("expected 1 port, got %d", len(alloc.AllocatedResources.Shared.Ports))
	}
}

func TestRestartAlloc(t *testing.T) {
	allocID := "alloc-abc123"
	taskName := "mc-survival"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/allocation/" + allocID:
			json.NewEncoder(w).Encode(&api.Allocation{ID: allocID, NodeID: "node-1"})
		case "/v1/client/allocation/" + allocID + "/restart":
			if r.Method != http.MethodPut {
				t.Errorf("method = %s, want PUT", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
		case "/v1/node/node-1":
			json.NewEncoder(w).Encode(&api.Node{ID: "node-1", HTTPAddr: "127.0.0.1:1"})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.RestartAlloc(allocID, taskName)
	if err != nil {
		t.Fatalf("RestartAlloc() error = %v", err)
	}
}

func TestGetJobVersions(t *testing.T) {
	jobID := "minecraft-survival"
	v0, v1 := uint64(0), uint64(1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/"+jobID+"/versions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Nomad returns {"Versions": [...], "Diffs": [...]}
		json.NewEncoder(w).Encode(map[string]any{
			"Versions": []*api.Job{
				{ID: &jobID, Version: &v1},
				{ID: &jobID, Version: &v0},
			},
			"Diffs": nil,
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	versions, err := c.GetJobVersions(jobID)
	if err != nil {
		t.Fatalf("GetJobVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("len(versions) = %d, want 2", len(versions))
	}
	if *versions[0].Version != 1 {
		t.Errorf("versions[0].Version = %d, want 1", *versions[0].Version)
	}
}

func TestRevertJob(t *testing.T) {
	jobID := "minecraft-survival"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/"+jobID+"/revert" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			t.Errorf("method = %s, want POST or PUT", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&api.JobRegisterResponse{EvalID: "eval-revert-1"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.RevertJob(jobID, 2)
	if err != nil {
		t.Fatalf("RevertJob() error = %v", err)
	}
	if resp.EvalID != "eval-revert-1" {
		t.Errorf("EvalID = %q, want eval-revert-1", resp.EvalID)
	}
}

func TestListNodePools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/node/pools" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*api.NodePool{
			{Name: "default"},
			{Name: "high-memory"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	pools, err := c.ListNodePools()
	if err != nil {
		t.Fatalf("ListNodePools() error = %v", err)
	}
	if len(pools) != 2 {
		t.Errorf("len(pools) = %d, want 2", len(pools))
	}
}

func TestListNodesInPool(t *testing.T) {
	poolName := "high-memory"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("node_pool") != poolName {
			t.Errorf("node_pool = %q, want %q", r.URL.Query().Get("node_pool"), poolName)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*api.NodeListStub{
			{ID: "node-1", Name: "worker-1", Status: "ready", NodePool: poolName},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	nodes, err := c.ListNodesInPool(poolName)
	if err != nil {
		t.Fatalf("ListNodesInPool() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("len(nodes) = %d, want 1", len(nodes))
	}
	if nodes[0].NodePool != poolName {
		t.Errorf("NodePool = %q, want %q", nodes[0].NodePool, poolName)
	}
}

func TestGetEvaluations(t *testing.T) {
	jobID := "minecraft-survival"
	want := []*api.Evaluation{
		{ID: "eval-1", JobID: jobID, Status: "complete"},
		{ID: "eval-2", JobID: jobID, Status: "blocked"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/"+jobID+"/evaluations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	evals, err := c.GetEvaluations(jobID)
	if err != nil {
		t.Fatalf("GetEvaluations() error = %v", err)
	}
	if len(evals) != 2 {
		t.Errorf("len(evals) = %d, want 2", len(evals))
	}
	if evals[0].ID != "eval-1" {
		t.Errorf("evals[0].ID = %q, want eval-1", evals[0].ID)
	}
}

func TestGetAllocations(t *testing.T) {
	jobID := "minecraft-survival"
	boolPtr := func(b bool) *bool { return &b }
	want := []*api.AllocationListStub{
		{
			ID:           "alloc-1",
			ClientStatus: "running",
			DeploymentStatus: &api.AllocDeploymentStatus{
				Healthy: boolPtr(true),
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/job/"+jobID+"/allocations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	allocs, err := c.GetAllocations(jobID)
	if err != nil {
		t.Fatalf("GetAllocations() error = %v", err)
	}
	if len(allocs) != 1 {
		t.Errorf("len(allocs) = %d, want 1", len(allocs))
	}
	if allocs[0].ID != "alloc-1" {
		t.Errorf("allocs[0].ID = %q, want alloc-1", allocs[0].ID)
	}
}

func TestGetAllocLogs(t *testing.T) {
	allocID := "alloc-abc123"
	taskName := "mc-survival"
	wantLog := "Server started on port 25565\nDone! For help, type \"help\"\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/allocation/" + allocID:
			json.NewEncoder(w).Encode(&api.Allocation{
				ID:        allocID,
				NodeID:    "node-1",
				Namespace: "default",
			})
		case "/v1/node/node-1":
			// Return a node with an unreachable HTTP address so the client
			// fast-fails the direct connection and falls back to server proxy.
			json.NewEncoder(w).Encode(&api.Node{
				ID:       "node-1",
				HTTPAddr: "127.0.0.1:1", // port 1 is unreachable
			})
		case "/v1/client/fs/logs/" + allocID:
			if r.URL.Query().Get("task") != taskName {
				t.Errorf("task = %q, want %q", r.URL.Query().Get("task"), taskName)
			}
			// Nomad streams newline-delimited JSON StreamFrame objects.
			// The Data field is base64-encoded by encoding/json since it's []byte.
			frame := api.StreamFrame{Data: []byte(wantLog)}
			json.NewEncoder(w).Encode(frame)
			// Closing the body causes the decoder to return io.EOF, which our
			// implementation treats as normal end-of-stream.
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	logs, err := c.GetAllocLogs(allocID, taskName, "stdout", "end", DefaultLogLimitBytes)
	if err != nil {
		t.Fatalf("GetAllocLogs() error = %v", err)
	}
	if logs != wantLog {
		t.Errorf("logs = %q, want %q", logs, wantLog)
	}
}

func TestGetJobSubmission(t *testing.T) {
	jobID := "minecraft-survival"
	version := uint64(3)
	wantSource := `job "minecraft-survival" { type = "service" }`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/job/" + jobID:
			json.NewEncoder(w).Encode(&api.Job{ID: &jobID, Version: &version})
		case "/v1/job/" + jobID + "/submission":
			if r.URL.Query().Get("version") != "3" {
				t.Errorf("submission version = %q, want %q", r.URL.Query().Get("version"), "3")
			}
			json.NewEncoder(w).Encode(&api.JobSubmission{
				Source: wantSource,
				Format: "hcl2",
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	sub, err := c.GetJobSubmission(jobID)
	if err != nil {
		t.Fatalf("GetJobSubmission() error = %v", err)
	}
	if sub.Source != wantSource {
		t.Errorf("Source = %q, want %q", sub.Source, wantSource)
	}
	if sub.Format != "hcl2" {
		t.Errorf("Format = %q, want %q", sub.Format, "hcl2")
	}
}

func TestStopJob(t *testing.T) {
	jobID := "minecraft-survival"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/v1/job/"+jobID {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		purge := r.URL.Query().Get("purge")
		if purge != "false" && purge != "" {
			t.Errorf("purge = %q, want false", purge)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"EvalID": "eval-123"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.StopJob(jobID, false)
	if err != nil {
		t.Fatalf("StopJob() error = %v", err)
	}
	if resp.EvalID != "eval-123" {
		t.Errorf("EvalID = %q, want %q", resp.EvalID, "eval-123")
	}

}

func TestStopJob_Purge(t *testing.T) {
	jobID := "minecraft-survival"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		purge := r.URL.Query().Get("purge")
		if purge != "true" {
			t.Errorf("purge = %q, want true", purge)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"EvalID": "eval-456"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.StopJob(jobID, true)
	if err != nil {
		t.Fatalf("StopJob(purge=true) error = %v", err)
	}
}

func TestWatchJobHealth_BecomesHealthy(t *testing.T) {
	jobID := "minecraft-survival"
	callCount := 0

	boolPtr := func(b bool) *bool { return &b }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		// First call: no allocations yet; second call: running and healthy
		if callCount < 2 {
			json.NewEncoder(w).Encode([]*api.AllocationListStub{})
			return
		}
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{
				ID:           "alloc-1",
				ClientStatus: "running",
				DeploymentStatus: &api.AllocDeploymentStatus{
					Healthy: boolPtr(true),
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthy, err := c.WatchJobHealth(ctx, jobID)
	if err != nil {
		t.Fatalf("WatchJobHealth() error = %v", err)
	}
	if !healthy {
		t.Error("WatchJobHealth() = false, want true")
	}
}

func TestWatchJobHealth_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Always return pending allocation
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{ID: "alloc-1", ClientStatus: "pending"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	healthy, err := c.WatchJobHealth(ctx, "minecraft-survival")
	if err != nil {
		t.Fatalf("WatchJobHealth() unexpected error = %v", err)
	}
	if healthy {
		t.Error("WatchJobHealth() = true, want false (timeout)")
	}
}

func TestWatchJobHealth_AllTerminal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{ID: "alloc-1", ClientStatus: "failed"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	_, err := c.WatchJobHealth(ctx, "minecraft-survival")
	if err == nil {
		t.Error("WatchJobHealth() expected error for all-terminal allocations, got nil")
	}
}

func TestWatchJobHealth_TerminalPlusHealthy(t *testing.T) {
	hTrue := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{ID: "alloc-old", ClientStatus: "complete"},
			{ID: "alloc-new", ClientStatus: "running", DeploymentStatus: &api.AllocDeploymentStatus{Healthy: &hTrue}},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	healthy, err := c.WatchJobHealth(ctx, "minecraft-survival")
	if err != nil {
		t.Errorf("WatchJobHealth() unexpected error: %v", err)
	}
	if !healthy {
		t.Error("WatchJobHealth() = false, want true (terminal skipped, running is healthy)")
	}
}

// --- Error path tests ---

func TestListJobs_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.ListJobs("")
	if err == nil {
		t.Error("ListJobs() expected error on 500, got nil")
	}
}

func TestGetAllocInfo_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetAllocInfo("nonexistent")
	if err == nil {
		t.Error("GetAllocInfo() expected error on 404, got nil")
	}
}

func TestRestartAlloc_InfoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.RestartAlloc("nonexistent", "task")
	if err == nil {
		t.Error("RestartAlloc() expected error when allocation lookup fails, got nil")
	}
}

func TestGetJobVersions_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetJobVersions("nonexistent")
	if err == nil {
		t.Error("GetJobVersions() expected error on 500, got nil")
	}
}

func TestRevertJob_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.RevertJob("nonexistent", 0)
	if err == nil {
		t.Error("RevertJob() expected error on 500, got nil")
	}
}

func TestListNodePools_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.ListNodePools()
	if err == nil {
		t.Error("ListNodePools() expected error on 500, got nil")
	}
}

func TestListNodesInPool_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.ListNodesInPool("nonexistent")
	if err == nil {
		t.Error("ListNodesInPool() expected error on 500, got nil")
	}
}

func TestGetEvaluations_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetEvaluations("nonexistent")
	if err == nil {
		t.Error("GetEvaluations() expected error on 500, got nil")
	}
}

func TestGetAllocations_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetAllocations("nonexistent")
	if err == nil {
		t.Error("GetAllocations() expected error on 500, got nil")
	}
}

func TestGetAllocLogs_AllocInfoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetAllocLogs("nonexistent", "task", "stdout", "end", DefaultLogLimitBytes)
	if err == nil {
		t.Error("GetAllocLogs() expected error when allocation lookup fails, got nil")
	}
}

func TestGetAllocLogs_OriginStart(t *testing.T) {
	allocID := "alloc-abc123"
	taskName := "mc-survival"
	wantLog := "Starting server\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/allocation/" + allocID:
			json.NewEncoder(w).Encode(&api.Allocation{
				ID:        allocID,
				NodeID:    "node-1",
				Namespace: "default",
			})
		case "/v1/node/node-1":
			json.NewEncoder(w).Encode(&api.Node{
				ID:       "node-1",
				HTTPAddr: "127.0.0.1:1",
			})
		case "/v1/client/fs/logs/" + allocID:
			frame := api.StreamFrame{Data: []byte(wantLog)}
			json.NewEncoder(w).Encode(frame)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	// origin=start with limitBytes=0 — should read from beginning with offset=0
	logs, err := c.GetAllocLogs(allocID, taskName, "stdout", "start", 0)
	if err != nil {
		t.Fatalf("GetAllocLogs(origin=start) error = %v", err)
	}
	if logs != wantLog {
		t.Errorf("logs = %q, want %q", logs, wantLog)
	}
}

func TestGetJobSubmission_InfoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetJobSubmission("nonexistent")
	if err == nil {
		t.Error("GetJobSubmission() expected error when job lookup fails, got nil")
	}
}

func TestGetJobSubmission_SubmissionError(t *testing.T) {
	jobID := "test-job"
	version := uint64(1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/job/" + jobID:
			json.NewEncoder(w).Encode(&api.Job{ID: &jobID, Version: &version})
		case "/v1/job/" + jobID + "/submission":
			http.Error(w, "submission not found", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetJobSubmission(jobID)
	if err == nil {
		t.Error("GetJobSubmission() expected error when submission lookup fails, got nil")
	}
}

func TestGetJobSubmission_NilVersion(t *testing.T) {
	jobID := "test-job"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/job/" + jobID:
			// Return a job without Version set (nil)
			json.NewEncoder(w).Encode(&api.Job{ID: &jobID})
		case "/v1/job/" + jobID + "/submission":
			if r.URL.Query().Get("version") != "0" {
				t.Errorf("submission version = %q, want %q", r.URL.Query().Get("version"), "0")
			}
			json.NewEncoder(w).Encode(&api.JobSubmission{
				Source: `job "test" {}`,
				Format: "hcl2",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	sub, err := c.GetJobSubmission(jobID)
	if err != nil {
		t.Fatalf("GetJobSubmission() error = %v", err)
	}
	if sub.Source != `job "test" {}` {
		t.Errorf("Source = %q, want %q", sub.Source, `job "test" {}`)
	}
}

func TestStopJob_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.StopJob("nonexistent", false)
	if err == nil {
		t.Error("StopJob() expected error on 500, got nil")
	}
}

func TestWatchJobHealth_ErrorFromCheckHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	_, err := c.WatchJobHealth(ctx, "test-job")
	if err == nil {
		t.Error("WatchJobHealth() expected error when checkJobHealth fails, got nil")
	}
}

func TestWatchJobHealth_NilDeploymentStatus(t *testing.T) {
	callCount := 0
	hTrue := true

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount < 2 {
			// First call: running but nil DeploymentStatus
			json.NewEncoder(w).Encode([]*api.AllocationListStub{
				{ID: "alloc-1", ClientStatus: "running", DeploymentStatus: nil},
			})
			return
		}
		// Second call: healthy
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{
				ID:               "alloc-1",
				ClientStatus:     "running",
				DeploymentStatus: &api.AllocDeploymentStatus{Healthy: &hTrue},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthy, err := c.WatchJobHealth(ctx, "test-job")
	if err != nil {
		t.Fatalf("WatchJobHealth() error = %v", err)
	}
	if !healthy {
		t.Error("WatchJobHealth() = false, want true")
	}
}

func TestWatchJobHealth_NilHealthyPointer(t *testing.T) {
	callCount := 0
	hTrue := true

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount < 2 {
			// First call: running with DeploymentStatus but nil Healthy pointer
			json.NewEncoder(w).Encode([]*api.AllocationListStub{
				{
					ID:               "alloc-1",
					ClientStatus:     "running",
					DeploymentStatus: &api.AllocDeploymentStatus{Healthy: nil},
				},
			})
			return
		}
		// Second call: healthy
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{
				ID:               "alloc-1",
				ClientStatus:     "running",
				DeploymentStatus: &api.AllocDeploymentStatus{Healthy: &hTrue},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthy, err := c.WatchJobHealth(ctx, "test-job")
	if err != nil {
		t.Fatalf("WatchJobHealth() error = %v", err)
	}
	if !healthy {
		t.Error("WatchJobHealth() = false, want true")
	}
}

func TestWatchJobHealth_UnhealthyDeployment(t *testing.T) {
	hFalse := false
	hTrue := true
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount < 2 {
			json.NewEncoder(w).Encode([]*api.AllocationListStub{
				{
					ID:               "alloc-1",
					ClientStatus:     "running",
					DeploymentStatus: &api.AllocDeploymentStatus{Healthy: &hFalse},
				},
			})
			return
		}
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{
				ID:               "alloc-1",
				ClientStatus:     "running",
				DeploymentStatus: &api.AllocDeploymentStatus{Healthy: &hTrue},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthy, err := c.WatchJobHealth(ctx, "test-job")
	if err != nil {
		t.Fatalf("WatchJobHealth() error = %v", err)
	}
	if !healthy {
		t.Error("WatchJobHealth() = false, want true")
	}
}

func TestWatchJobHealth_LostAllocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]*api.AllocationListStub{
			{ID: "alloc-1", ClientStatus: "lost"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	_, err := c.WatchJobHealth(ctx, "test-job")
	if err == nil {
		t.Error("WatchJobHealth() expected error for all-lost allocations, got nil")
	}
}

func TestNewClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := NewClient(srv.URL, "test-token", logger)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c == nil {
		t.Fatal("NewClient() returned nil client")
	}
	if c.healthPollInterval != defaultHealthPollInterval {
		t.Errorf("healthPollInterval = %v, want %v", c.healthPollInterval, defaultHealthPollInterval)
	}
}

// strPtr is a test helper for *string literals.
func strPtr(s string) *string { return &s }
