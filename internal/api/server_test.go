package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	nomadapi "github.com/hashicorp/nomad/api"

	"github.com/lobo235/nomad-gateway/internal/api"
	"github.com/lobo235/nomad-gateway/internal/nomad"
)

const testAPIKey = "test-api-key"
const testVersion = "v1.0.0-test"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockNomad is a configurable mock satisfying the nomadClient interface.
type mockNomad struct {
	pingFunc           func() error
	listJobsFunc       func(prefix string) ([]*nomadapi.JobListStub, error)
	getJobFunc         func(jobID string) (*nomadapi.Job, error)
	getJobSubFunc      func(jobID string) (*nomadapi.JobSubmission, error)
	submitJobFunc      func(hclSpec string) (*nomadapi.JobRegisterResponse, error)
	planJobFunc        func(hclSpec string) (*nomadapi.JobPlanResponse, error)
	stopJobFunc        func(jobID string, purge bool) (*nomad.StopJobResponse, error)
	getAllocInfoFunc   func(allocID string) (*nomadapi.Allocation, error)
	restartAllocFunc   func(allocID, taskName string) error
	getJobVersionsFunc func(jobID string) ([]*nomadapi.Job, error)
	revertJobFunc      func(jobID string, version uint64) (*nomadapi.JobRegisterResponse, error)
	listNodePoolsFunc  func() ([]*nomadapi.NodePool, error)
	listNodesFunc      func(poolName string) ([]*nomadapi.NodeListStub, error)
	getEvalsFunc       func(jobID string) ([]*nomadapi.Evaluation, error)
	getAllocsFunc      func(jobID string) ([]*nomadapi.AllocationListStub, error)
	getLogsFunc        func(allocID, task, logType, origin string, limitBytes int64) (string, error)
	watchHealthFunc    func(ctx context.Context, jobID string) (bool, error)
}

func (m *mockNomad) Ping() error {
	if m.pingFunc != nil {
		return m.pingFunc()
	}
	return nil
}
func (m *mockNomad) ListJobs(prefix string) ([]*nomadapi.JobListStub, error) {
	if m.listJobsFunc != nil {
		return m.listJobsFunc(prefix)
	}
	return nil, nil
}
func (m *mockNomad) GetJob(jobID string) (*nomadapi.Job, error) {
	if m.getJobFunc != nil {
		return m.getJobFunc(jobID)
	}
	return nil, nil
}
func (m *mockNomad) GetJobSubmission(jobID string) (*nomadapi.JobSubmission, error) {
	if m.getJobSubFunc != nil {
		return m.getJobSubFunc(jobID)
	}
	return nil, nil
}
func (m *mockNomad) SubmitJob(hclSpec string) (*nomadapi.JobRegisterResponse, error) {
	if m.submitJobFunc != nil {
		return m.submitJobFunc(hclSpec)
	}
	return nil, nil
}
func (m *mockNomad) PlanJob(hclSpec string) (*nomadapi.JobPlanResponse, error) {
	if m.planJobFunc != nil {
		return m.planJobFunc(hclSpec)
	}
	return nil, nil
}
func (m *mockNomad) StopJob(jobID string, purge bool) (*nomad.StopJobResponse, error) {
	if m.stopJobFunc != nil {
		return m.stopJobFunc(jobID, purge)
	}
	return &nomad.StopJobResponse{}, nil
}
func (m *mockNomad) GetAllocInfo(allocID string) (*nomadapi.Allocation, error) {
	if m.getAllocInfoFunc != nil {
		return m.getAllocInfoFunc(allocID)
	}
	return nil, nil
}
func (m *mockNomad) RestartAlloc(allocID, taskName string) error {
	if m.restartAllocFunc != nil {
		return m.restartAllocFunc(allocID, taskName)
	}
	return nil
}
func (m *mockNomad) GetJobVersions(jobID string) ([]*nomadapi.Job, error) {
	if m.getJobVersionsFunc != nil {
		return m.getJobVersionsFunc(jobID)
	}
	return nil, nil
}
func (m *mockNomad) RevertJob(jobID string, version uint64) (*nomadapi.JobRegisterResponse, error) {
	if m.revertJobFunc != nil {
		return m.revertJobFunc(jobID, version)
	}
	return nil, nil
}
func (m *mockNomad) ListNodePools() ([]*nomadapi.NodePool, error) {
	if m.listNodePoolsFunc != nil {
		return m.listNodePoolsFunc()
	}
	return nil, nil
}
func (m *mockNomad) ListNodesInPool(poolName string) ([]*nomadapi.NodeListStub, error) {
	if m.listNodesFunc != nil {
		return m.listNodesFunc(poolName)
	}
	return nil, nil
}
func (m *mockNomad) GetEvaluations(jobID string) ([]*nomadapi.Evaluation, error) {
	if m.getEvalsFunc != nil {
		return m.getEvalsFunc(jobID)
	}
	return nil, nil
}
func (m *mockNomad) GetAllocations(jobID string) ([]*nomadapi.AllocationListStub, error) {
	if m.getAllocsFunc != nil {
		return m.getAllocsFunc(jobID)
	}
	return nil, nil
}
func (m *mockNomad) GetAllocLogs(allocID, task, logType, origin string, limitBytes int64) (string, error) {
	if m.getLogsFunc != nil {
		return m.getLogsFunc(allocID, task, logType, origin, limitBytes)
	}
	return "", nil
}
func (m *mockNomad) WatchJobHealth(ctx context.Context, jobID string) (bool, error) {
	if m.watchHealthFunc != nil {
		return m.watchHealthFunc(ctx, jobID)
	}
	return true, nil
}

// newTestServer wires a mock nomad client into a test HTTP server.
func newTestServer(t *testing.T, mock *mockNomad) *httptest.Server {
	t.Helper()
	srv := api.NewServer(mock, testAPIKey, testVersion, discardLogger())
	return httptest.NewServer(srv.Handler())
}

func authHeader() string { return "Bearer " + testAPIKey }

func doRequest(t *testing.T, method, url, body, auth string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}
	req, _ := http.NewRequest(method, url, bodyReader)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Errorf("status = %d, want %d", resp.StatusCode, want)
	}
}

func assertErrorCode(t *testing.T, resp *http.Response, wantCode string) {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Code != wantCode {
		t.Errorf("error code = %q, want %q", body.Code, wantCode)
	}
}

// --- auth middleware ---

func TestAuth_MissingToken(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs", "", "")
	assertStatus(t, resp, http.StatusUnauthorized)
	assertErrorCode(t, resp, "unauthorized")
}

func TestAuth_WrongToken(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs", "", "Bearer wrong")
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestAuth_ValidToken(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		listJobsFunc: func(prefix string) ([]*nomadapi.JobListStub, error) {
			return []*nomadapi.JobListStub{}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

// --- GET /health ---

func TestHealth_NomadUp(t *testing.T) {
	srv := newTestServer(t, &mockNomad{pingFunc: func() error { return nil }})
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/health", "", "")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if body.Version != testVersion {
		t.Errorf("version = %q, want %q", body.Version, testVersion)
	}
}

func TestHealth_NomadDown(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		pingFunc: func() error { return errors.New("connection refused") },
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/health", "", "")
	assertStatus(t, resp, http.StatusServiceUnavailable)
}

func TestHealth_NoAuthRequired(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/health", "", "")
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("/health should not require auth")
	}
}

// --- GET /jobs ---

func TestListJobs_OK(t *testing.T) {
	id := "my-job"
	srv := newTestServer(t, &mockNomad{
		listJobsFunc: func(prefix string) ([]*nomadapi.JobListStub, error) {
			return []*nomadapi.JobListStub{{ID: id}}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestListJobs_FilterPropagated(t *testing.T) {
	var gotPrefix string
	srv := newTestServer(t, &mockNomad{
		listJobsFunc: func(prefix string) ([]*nomadapi.JobListStub, error) {
			gotPrefix = prefix
			return nil, nil
		},
	})
	defer srv.Close()
	doRequest(t, http.MethodGet, srv.URL+"/jobs?filter=web", "", authHeader())
	if gotPrefix != "web" {
		t.Errorf("prefix = %q, want web", gotPrefix)
	}
}

func TestListJobs_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		listJobsFunc: func(prefix string) ([]*nomadapi.JobListStub, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- POST /jobs ---

func TestSubmitJob_EmptyBody(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/jobs", bytes.NewBufferString(""))
	req.Header.Set("Authorization", authHeader())
	resp, _ := http.DefaultClient.Do(req)
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "empty_body")
}

func TestSubmitJob_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		submitJobFunc: func(hclSpec string) (*nomadapi.JobRegisterResponse, error) {
			return &nomadapi.JobRegisterResponse{}, nil
		},
	})
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/jobs", bytes.NewBufferString(`job "test" {}`))
	req.Header.Set("Authorization", authHeader())
	resp, _ := http.DefaultClient.Do(req)
	assertStatus(t, resp, http.StatusOK)
}

// --- DELETE /jobs/{jobID} ---

func TestStopJob_InvalidPurgeParam(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodDelete, srv.URL+"/jobs/my-job?purge=notbool", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "invalid_param")
}

func TestStopJob_OK(t *testing.T) {
	var gotPurge bool
	srv := newTestServer(t, &mockNomad{
		stopJobFunc: func(jobID string, purge bool) (*nomad.StopJobResponse, error) {
			gotPurge = purge
			return &nomad.StopJobResponse{EvalID: "eval-1"}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodDelete, srv.URL+"/jobs/my-job?purge=true", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
	if !gotPurge {
		t.Error("expected purge=true to be passed to StopJob")
	}
}

// --- POST /jobs/{jobID}/revert ---

func TestRevertJob_MissingVersion(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodPost, srv.URL+"/jobs/my-job/revert", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "missing_param")
}

func TestRevertJob_InvalidVersion(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodPost, srv.URL+"/jobs/my-job/revert?version=abc", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "invalid_param")
}

func TestRevertJob_OK(t *testing.T) {
	var gotVersion uint64
	srv := newTestServer(t, &mockNomad{
		revertJobFunc: func(jobID string, version uint64) (*nomadapi.JobRegisterResponse, error) {
			gotVersion = version
			return &nomadapi.JobRegisterResponse{}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodPost, srv.URL+"/jobs/my-job/revert?version=3", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
	if gotVersion != 3 {
		t.Errorf("version = %d, want 3", gotVersion)
	}
}

// --- GET /jobs/{jobID}/allocations/{allocID}/logs ---

func TestGetLogs_MissingTask(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "missing_param")
}

func TestGetLogs_InvalidType(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web&type=badtype", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "invalid_param")
}

func TestGetLogs_InvalidOrigin(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web&origin=middle", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "invalid_param")
}

func TestGetLogs_InvalidLimit(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web&limit=-1", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "invalid_param")
}

func TestGetLogs_LimitZeroForcesOriginStart(t *testing.T) {
	var gotOrigin string
	srv := newTestServer(t, &mockNomad{
		getLogsFunc: func(allocID, task, logType, origin string, limitBytes int64) (string, error) {
			gotOrigin = origin
			return "log output", nil
		},
	})
	defer srv.Close()
	doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web&limit=0", "", authHeader())
	if gotOrigin != "start" {
		t.Errorf("origin = %q, want start when limit=0", gotOrigin)
	}
}

func TestGetLogs_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getLogsFunc: func(allocID, task, logType, origin string, limitBytes int64) (string, error) {
			return "log line 1\nlog line 2\n", nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "log line 1\nlog line 2\n" {
		t.Errorf("unexpected log body: %q", string(body))
	}
}

// --- GET /jobs/{jobID}/health ---

func TestWatchJobHealth_InvalidTimeout(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/health?timeout=notaduration", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "invalid_param")
}

func TestWatchJobHealth_Timeout(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		watchHealthFunc: func(ctx context.Context, jobID string) (bool, error) {
			return false, nil // simulate timeout (WatchJobHealth returns false)
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/health?timeout=1ms", "", authHeader())
	assertStatus(t, resp, http.StatusRequestTimeout)
	assertErrorCode(t, resp, "timeout")
}

func TestWatchJobHealth_Healthy(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		watchHealthFunc: func(ctx context.Context, jobID string) (bool, error) {
			return true, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/health", "", authHeader())
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		JobID   string `json:"job_id"`
		Healthy bool   `json:"healthy"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if !body.Healthy {
		t.Error("expected healthy=true")
	}
	if body.JobID != "my-job" {
		t.Errorf("job_id = %q, want my-job", body.JobID)
	}
}

func TestWatchJobHealth_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		watchHealthFunc: func(ctx context.Context, jobID string) (bool, error) {
			return false, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/health", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /jobs/{jobID} ---

func TestGetJob_OK(t *testing.T) {
	jobID := "my-job"
	srv := newTestServer(t, &mockNomad{
		getJobFunc: func(id string) (*nomadapi.Job, error) {
			return &nomadapi.Job{ID: &jobID}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestGetJob_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getJobFunc: func(id string) (*nomadapi.Job, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /jobs/{jobID}/spec ---

func TestGetJobSpec_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getJobSubFunc: func(jobID string) (*nomadapi.JobSubmission, error) {
			return &nomadapi.JobSubmission{Source: `job "test" {}`, Format: "hcl2"}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/spec", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestGetJobSpec_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getJobSubFunc: func(jobID string) (*nomadapi.JobSubmission, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/spec", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /jobs/{jobID}/allocations/{allocID} ---

func TestGetAllocInfo_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getAllocInfoFunc: func(allocID string) (*nomadapi.Allocation, error) {
			return &nomadapi.Allocation{ID: allocID}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestGetAllocInfo_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getAllocInfoFunc: func(allocID string) (*nomadapi.Allocation, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- POST /jobs/{jobID}/allocations/{allocID}/restart ---

func TestRestartAlloc_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		restartAllocFunc: func(allocID, taskName string) error {
			return nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodPost, srv.URL+"/jobs/my-job/allocations/alloc-1/restart?task=web", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestRestartAlloc_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		restartAllocFunc: func(allocID, taskName string) error {
			return errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodPost, srv.URL+"/jobs/my-job/allocations/alloc-1/restart", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

func TestRestartAlloc_TaskParam(t *testing.T) {
	var gotTask string
	srv := newTestServer(t, &mockNomad{
		restartAllocFunc: func(allocID, taskName string) error {
			gotTask = taskName
			return nil
		},
	})
	defer srv.Close()
	doRequest(t, http.MethodPost, srv.URL+"/jobs/my-job/allocations/alloc-1/restart?task=mc-server", "", authHeader())
	if gotTask != "mc-server" {
		t.Errorf("task = %q, want mc-server", gotTask)
	}
}

// --- GET /jobs/{jobID}/versions ---

func TestGetJobVersions_OK(t *testing.T) {
	jobID := "my-job"
	v0 := uint64(0)
	srv := newTestServer(t, &mockNomad{
		getJobVersionsFunc: func(id string) ([]*nomadapi.Job, error) {
			return []*nomadapi.Job{{ID: &jobID, Version: &v0}}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/versions", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestGetJobVersions_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getJobVersionsFunc: func(id string) ([]*nomadapi.Job, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/versions", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /node-pools ---

func TestListNodePools_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		listNodePoolsFunc: func() ([]*nomadapi.NodePool, error) {
			return []*nomadapi.NodePool{{Name: "default"}}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/node-pools", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestListNodePools_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		listNodePoolsFunc: func() ([]*nomadapi.NodePool, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/node-pools", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /node-pools/{poolName}/nodes ---

func TestListNodesInPool_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		listNodesFunc: func(poolName string) ([]*nomadapi.NodeListStub, error) {
			return []*nomadapi.NodeListStub{{ID: "node-1", NodePool: poolName}}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/node-pools/default/nodes", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestListNodesInPool_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		listNodesFunc: func(poolName string) ([]*nomadapi.NodeListStub, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/node-pools/default/nodes", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

func TestListNodesInPool_PoolNamePropagated(t *testing.T) {
	var gotPool string
	srv := newTestServer(t, &mockNomad{
		listNodesFunc: func(poolName string) ([]*nomadapi.NodeListStub, error) {
			gotPool = poolName
			return nil, nil
		},
	})
	defer srv.Close()
	doRequest(t, http.MethodGet, srv.URL+"/node-pools/high-memory/nodes", "", authHeader())
	if gotPool != "high-memory" {
		t.Errorf("pool = %q, want high-memory", gotPool)
	}
}

// --- GET /jobs/{jobID}/evaluations ---

func TestGetEvaluations_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getEvalsFunc: func(jobID string) ([]*nomadapi.Evaluation, error) {
			return []*nomadapi.Evaluation{{ID: "eval-1", JobID: jobID}}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/evaluations", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestGetEvaluations_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getEvalsFunc: func(jobID string) ([]*nomadapi.Evaluation, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/evaluations", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /jobs/{jobID}/allocations ---

func TestGetAllocations_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getAllocsFunc: func(jobID string) ([]*nomadapi.AllocationListStub, error) {
			return []*nomadapi.AllocationListStub{{ID: "alloc-1"}}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}

func TestGetAllocations_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getAllocsFunc: func(jobID string) ([]*nomadapi.AllocationListStub, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- POST /jobs (submit) upstream error ---

func TestSubmitJob_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		submitJobFunc: func(hclSpec string) (*nomadapi.JobRegisterResponse, error) {
			return nil, errors.New("parse error")
		},
	})
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/jobs", bytes.NewBufferString(`job "test" {}`))
	req.Header.Set("Authorization", authHeader())
	resp, _ := http.DefaultClient.Do(req)
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- DELETE /jobs/{jobID} upstream error ---

func TestStopJob_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		stopJobFunc: func(jobID string, purge bool) (*nomad.StopJobResponse, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodDelete, srv.URL+"/jobs/my-job", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- POST /jobs/{jobID}/revert upstream error ---

func TestRevertJob_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		revertJobFunc: func(jobID string, version uint64) (*nomadapi.JobRegisterResponse, error) {
			return nil, errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodPost, srv.URL+"/jobs/my-job/revert?version=1", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /jobs/{jobID}/allocations/{allocID}/logs upstream error ---

func TestGetLogs_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		getLogsFunc: func(allocID, task, logType, origin string, limitBytes int64) (string, error) {
			return "", errors.New("nomad unavailable")
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web", "", authHeader())
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- GET /jobs/{jobID}/allocations/{allocID}/logs defaults ---

func TestGetLogs_Defaults(t *testing.T) {
	var gotLogType, gotOrigin string
	var gotLimit int64
	srv := newTestServer(t, &mockNomad{
		getLogsFunc: func(allocID, task, logType, origin string, limitBytes int64) (string, error) {
			gotLogType = logType
			gotOrigin = origin
			gotLimit = limitBytes
			return "output", nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
	if gotLogType != "stdout" {
		t.Errorf("logType = %q, want stdout", gotLogType)
	}
	if gotOrigin != "end" {
		t.Errorf("origin = %q, want end", gotOrigin)
	}
	if gotLimit != int64(nomad.DefaultLogLimitBytes) {
		t.Errorf("limit = %d, want %d", gotLimit, nomad.DefaultLogLimitBytes)
	}
}

func TestGetLogs_StderrOriginStart(t *testing.T) {
	var gotLogType, gotOrigin string
	srv := newTestServer(t, &mockNomad{
		getLogsFunc: func(allocID, task, logType, origin string, limitBytes int64) (string, error) {
			gotLogType = logType
			gotOrigin = origin
			return "error output", nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web&type=stderr&origin=start", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
	if gotLogType != "stderr" {
		t.Errorf("logType = %q, want stderr", gotLogType)
	}
	if gotOrigin != "start" {
		t.Errorf("origin = %q, want start", gotOrigin)
	}
}

func TestGetLogs_InvalidLimitNotANumber(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/allocations/alloc-1/logs?task=web&limit=abc", "", authHeader())
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "invalid_param")
}

// --- POST /jobs/plan ---

func TestPlanJob_EmptyBody(t *testing.T) {
	srv := newTestServer(t, &mockNomad{})
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/jobs/plan", bytes.NewBufferString(""))
	req.Header.Set("Authorization", authHeader())
	resp, _ := http.DefaultClient.Do(req)
	assertStatus(t, resp, http.StatusBadRequest)
	assertErrorCode(t, resp, "empty_body")
}

func TestPlanJob_OK(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		planJobFunc: func(hclSpec string) (*nomadapi.JobPlanResponse, error) {
			return &nomadapi.JobPlanResponse{
				Warnings:       "some warning",
				JobModifyIndex: 42,
			}, nil
		},
	})
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/jobs/plan", bytes.NewBufferString(`job "test" {}`))
	req.Header.Set("Authorization", authHeader())
	resp, _ := http.DefaultClient.Do(req)
	assertStatus(t, resp, http.StatusOK)

	var body nomadapi.JobPlanResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.JobModifyIndex != 42 {
		t.Errorf("JobModifyIndex = %d, want 42", body.JobModifyIndex)
	}
	if body.Warnings != "some warning" {
		t.Errorf("Warnings = %q, want 'some warning'", body.Warnings)
	}
}

func TestPlanJob_UpstreamError(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		planJobFunc: func(hclSpec string) (*nomadapi.JobPlanResponse, error) {
			return nil, errors.New("parse error")
		},
	})
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/jobs/plan", bytes.NewBufferString(`job "test" {}`))
	req.Header.Set("Authorization", authHeader())
	resp, _ := http.DefaultClient.Do(req)
	assertStatus(t, resp, http.StatusBadGateway)
	assertErrorCode(t, resp, "nomad_error")
}

// --- DELETE /jobs/{jobID} default purge ---

func TestStopJob_DefaultPurge(t *testing.T) {
	var gotPurge bool
	srv := newTestServer(t, &mockNomad{
		stopJobFunc: func(jobID string, purge bool) (*nomad.StopJobResponse, error) {
			gotPurge = purge
			return &nomad.StopJobResponse{EvalID: "eval-1"}, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodDelete, srv.URL+"/jobs/my-job", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
	if gotPurge {
		t.Error("expected purge=false by default")
	}
}

// --- GET /jobs/{jobID}/health default timeout ---

func TestWatchJobHealth_DefaultTimeout(t *testing.T) {
	srv := newTestServer(t, &mockNomad{
		watchHealthFunc: func(ctx context.Context, jobID string) (bool, error) {
			return true, nil
		},
	})
	defer srv.Close()
	resp := doRequest(t, http.MethodGet, srv.URL+"/jobs/my-job/health", "", authHeader())
	assertStatus(t, resp, http.StatusOK)
}
