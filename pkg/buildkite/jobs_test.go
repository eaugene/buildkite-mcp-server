package buildkite

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetJobs(t *testing.T) {
	ctx := context.Background()
	client := &MockBuildsClient{
		GetFunc: func(ctx context.Context, org string, pipeline string, id string, opt *buildkite.BuildGetOptions) (buildkite.Build, *buildkite.Response, error) {
			return buildkite.Build{
					ID:        "123",
					Number:    1,
					State:     "finished",
					CreatedAt: &buildkite.Timestamp{},
					Jobs: []buildkite.Job{
						{ID: "job1", State: "passed", Agent: buildkite.Agent{ID: "agent1", Name: "test-agent-1"}},
						{ID: "job2", State: "failed", Agent: buildkite.Agent{ID: "agent2", Name: "test-agent-2"}},
						{ID: "job3", State: "running", Agent: buildkite.Agent{ID: "agent3", Name: "test-agent-3"}},
						{ID: "job4", State: "waiting"},
					},
				}, &buildkite.Response{
					Response: &http.Response{
						StatusCode: 200,
					},
				}, nil
		},
	}

	tool, handler, _ := GetJobs(client)
	require.NotNil(t, tool)
	require.NotNil(t, handler)

	// Test getting all jobs (no filter) - agent info should be excluded by default
	requestAll := createMCPRequest(t, map[string]any{})
	argsAll := GetJobsArgs{
		OrgSlug:      "org",
		PipelineSlug: "pipeline",
		BuildNumber:  "1",
	}
	resultAll, err := handler(ctx, requestAll, argsAll)
	require.NoError(t, err)

	textContentAll := getTextResult(t, resultAll)
	// All jobs should be returned
	assert.Contains(t, textContentAll.Text, `"job1"`)
	assert.Contains(t, textContentAll.Text, `"job2"`)
	assert.Contains(t, textContentAll.Text, `"job3"`)
	assert.Contains(t, textContentAll.Text, `"job4"`)
	// Agent ID should be included by default but not detailed info
	assert.Contains(t, textContentAll.Text, `"agent1"`)
	assert.NotContains(t, textContentAll.Text, `"test-agent-1"`)
	assert.Contains(t, textContentAll.Text, `"agent2"`)
	assert.NotContains(t, textContentAll.Text, `"test-agent-2"`)
	assert.Contains(t, textContentAll.Text, `"agent3"`)
	assert.NotContains(t, textContentAll.Text, `"test-agent-3"`)
	// Should always have pagination metadata (default page size 30)
	assert.Contains(t, textContentAll.Text, `"page":1`)
	assert.Contains(t, textContentAll.Text, `"per_page":30`)
	assert.Contains(t, textContentAll.Text, `"total":4`)
	assert.Contains(t, textContentAll.Text, `"has_next":false`)
	assert.Contains(t, textContentAll.Text, `"has_prev":false`)
}

func TestGetJobsWithStateFilter(t *testing.T) {
	ctx := context.Background()
	client := &MockBuildsClient{
		GetFunc: func(ctx context.Context, org string, pipeline string, id string, opt *buildkite.BuildGetOptions) (buildkite.Build, *buildkite.Response, error) {
			// Create a build with various job states
			return buildkite.Build{
					ID:        "123",
					Number:    1,
					State:     "finished",
					CreatedAt: &buildkite.Timestamp{},
					Jobs: []buildkite.Job{
						{ID: "job1", State: "passed", Agent: buildkite.Agent{ID: "agent1", Name: "test-agent-1"}},
						{ID: "job2", State: "failed", Agent: buildkite.Agent{ID: "agent2", Name: "test-agent-2"}},
						{ID: "job3", State: "running", Agent: buildkite.Agent{ID: "agent3", Name: "test-agent-3"}},
						{ID: "job4", State: "waiting"},
						{ID: "job5", State: "passed", Agent: buildkite.Agent{ID: "agent5", Name: "test-agent-5"}},
						{ID: "job6", State: "canceled"},
					},
				}, &buildkite.Response{
					Response: &http.Response{
						StatusCode: 200,
					},
				}, nil
		},
	}

	tool, handler, _ := GetJobs(client)
	require.NotNil(t, tool)
	require.NotNil(t, handler)

	t.Run("filter by passed state", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			JobState:     "passed",
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		// Only filtered jobs are returned
		assert.Contains(t, textContent.Text, `"job1"`)
		assert.Contains(t, textContent.Text, `"job5"`)
		assert.NotContains(t, textContent.Text, `"job2"`)
		assert.NotContains(t, textContent.Text, `"job3"`)
		assert.NotContains(t, textContent.Text, `"job4"`)
		assert.NotContains(t, textContent.Text, `"job6"`)
		// Should always have pagination metadata (default page size 30)
		assert.Contains(t, textContent.Text, `"page":1`)
		assert.Contains(t, textContent.Text, `"per_page":30`)
		assert.Contains(t, textContent.Text, `"total":2`)
		assert.Contains(t, textContent.Text, `"has_next":false`)
		assert.Contains(t, textContent.Text, `"has_prev":false`)
	})

	t.Run("filter by failed state", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			JobState:     "failed",
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		// Only filtered jobs are returned
		assert.Contains(t, textContent.Text, `"job2"`)
		assert.NotContains(t, textContent.Text, `"job1"`)
		assert.NotContains(t, textContent.Text, `"job3"`)
		assert.NotContains(t, textContent.Text, `"job4"`)
		assert.NotContains(t, textContent.Text, `"job5"`)
		assert.NotContains(t, textContent.Text, `"job6"`)
		// Should always have pagination metadata (default page size 30)
		assert.Contains(t, textContent.Text, `"page":1`)
		assert.Contains(t, textContent.Text, `"per_page":30`)
		assert.Contains(t, textContent.Text, `"total":1`)
		assert.Contains(t, textContent.Text, `"has_next":false`)
		assert.Contains(t, textContent.Text, `"has_prev":false`)
	})

	t.Run("filter by running state", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			JobState:     "running",
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		// Only filtered jobs are returned
		assert.Contains(t, textContent.Text, `"job3"`)
		assert.NotContains(t, textContent.Text, `"job1"`)
		assert.NotContains(t, textContent.Text, `"job2"`)
		assert.NotContains(t, textContent.Text, `"job4"`)
		assert.NotContains(t, textContent.Text, `"job5"`)
		assert.NotContains(t, textContent.Text, `"job6"`)
		// Should always have pagination metadata (default page size 30)
		assert.Contains(t, textContent.Text, `"page":1`)
		assert.Contains(t, textContent.Text, `"per_page":30`)
		assert.Contains(t, textContent.Text, `"total":1`)
		assert.Contains(t, textContent.Text, `"has_next":false`)
		assert.Contains(t, textContent.Text, `"has_prev":false`)
	})
}

func TestGetJobsMissingParameters(t *testing.T) {
	ctx := context.Background()
	client := &MockBuildsClient{}

	tool, handler, _ := GetJobs(client)
	require.NotNil(t, tool)
	require.NotNil(t, handler)

	t.Run("missing org", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Content, 1)
		errorContent, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, errorContent.Text, "org")
	})

	t.Run("missing pipeline_slug", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:     "org",
			BuildNumber: "1",
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Content, 1)
		errorContent, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, errorContent.Text, "pipeline_slug")
	})

	t.Run("missing build_number", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Content, 1)
		errorContent, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, errorContent.Text, "build_number")
	})
}

func TestGetJobsPagination(t *testing.T) {
	ctx := context.Background()
	client := &MockBuildsClient{
		GetFunc: func(ctx context.Context, org string, pipeline string, id string, opt *buildkite.BuildGetOptions) (buildkite.Build, *buildkite.Response, error) {
			// Create a build with 6 jobs to test pagination
			return buildkite.Build{
					ID:        "123",
					Number:    1,
					State:     "finished",
					CreatedAt: &buildkite.Timestamp{},
					Jobs: []buildkite.Job{
						{ID: "job1", State: "passed", Agent: buildkite.Agent{ID: "agent1", Name: "test-agent-1"}},
						{ID: "job2", State: "failed", Agent: buildkite.Agent{ID: "agent2", Name: "test-agent-2"}},
						{ID: "job3", State: "running", Agent: buildkite.Agent{ID: "agent3", Name: "test-agent-3"}},
						{ID: "job4", State: "waiting"},
						{ID: "job5", State: "passed", Agent: buildkite.Agent{ID: "agent5", Name: "test-agent-5"}},
						{ID: "job6", State: "canceled"},
					},
				}, &buildkite.Response{
					Response: &http.Response{
						StatusCode: 200,
					},
				}, nil
		},
	}

	tool, handler, _ := GetJobs(client)
	require.NotNil(t, tool)
	require.NotNil(t, handler)

	t.Run("first page", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			Page:         1,
			PerPage:      2,
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		// Should contain first 2 jobs
		assert.Contains(t, textContent.Text, `"job1"`)
		assert.Contains(t, textContent.Text, `"job2"`)
		assert.NotContains(t, textContent.Text, `"job3"`)
		assert.NotContains(t, textContent.Text, `"job4"`)
		// Should have pagination metadata
		assert.Contains(t, textContent.Text, `"page":1`)
		assert.Contains(t, textContent.Text, `"per_page":2`)
		assert.Contains(t, textContent.Text, `"total":6`)
		assert.Contains(t, textContent.Text, `"has_next":true`)
		assert.Contains(t, textContent.Text, `"has_prev":false`)
	})

	t.Run("second page", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			Page:         2,
			PerPage:      2,
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		// Should contain next 2 jobs
		assert.NotContains(t, textContent.Text, `"job1"`)
		assert.NotContains(t, textContent.Text, `"job2"`)
		assert.Contains(t, textContent.Text, `"job3"`)
		assert.Contains(t, textContent.Text, `"job4"`)
		// Should have pagination metadata
		assert.Contains(t, textContent.Text, `"page":2`)
		assert.Contains(t, textContent.Text, `"per_page":2`)
		assert.Contains(t, textContent.Text, `"total":6`)
		assert.Contains(t, textContent.Text, `"has_next":true`)
		assert.Contains(t, textContent.Text, `"has_prev":true`)
	})

	t.Run("last page", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			Page:         3,
			PerPage:      2,
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		// Should contain last 2 jobs
		assert.Contains(t, textContent.Text, `"job5"`)
		assert.Contains(t, textContent.Text, `"job6"`)
		// Should have pagination metadata
		assert.Contains(t, textContent.Text, `"page":3`)
		assert.Contains(t, textContent.Text, `"per_page":2`)
		assert.Contains(t, textContent.Text, `"total":6`)
		assert.Contains(t, textContent.Text, `"has_next":false`)
		assert.Contains(t, textContent.Text, `"has_prev":true`)
	})

	t.Run("page beyond available data", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			Page:         5,
			PerPage:      2,
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		// Should contain empty items array
		assert.Contains(t, textContent.Text, `"items":[]`)
	})
}

func TestGetJobsAgentInfo(t *testing.T) {
	ctx := context.Background()
	client := &MockBuildsClient{
		GetFunc: func(ctx context.Context, org string, pipeline string, id string, opt *buildkite.BuildGetOptions) (buildkite.Build, *buildkite.Response, error) {
			// Create a build with jobs that have agent info
			return buildkite.Build{
					ID:        "123",
					Number:    1,
					State:     "finished",
					CreatedAt: &buildkite.Timestamp{},
					Jobs: []buildkite.Job{
						{ID: "job1", State: "passed", Agent: buildkite.Agent{ID: "agent1", Name: "test-agent-1"}},
						{ID: "job2", State: "running", Agent: buildkite.Agent{ID: "agent2", Name: "test-agent-2"}},
						{ID: "job3", State: "waiting"}, // no agent
					},
				}, &buildkite.Response{
					Response: &http.Response{
						StatusCode: 200,
					},
				}, nil
		},
	}

	tool, handler, _ := GetJobs(client)
	require.NotNil(t, tool)
	require.NotNil(t, handler)

	t.Run("default behavior excludes detailed agent info", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		assert.Contains(t, textContent.Text, `"job1"`)
		// By default, should include agent ID but not detailed info
		assert.Contains(t, textContent.Text, `"agent1"`)
		assert.NotContains(t, textContent.Text, `"test-agent-1"`)
	})

	t.Run("include_agent=false excludes detailed agent info", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			IncludeAgent: false,
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		assert.Contains(t, textContent.Text, `"job1"`)
		assert.Contains(t, textContent.Text, `"job2"`)
		assert.Contains(t, textContent.Text, `"job3"`)
		// Agent IDs should be included but not detailed info
		assert.Contains(t, textContent.Text, `"agent1"`)
		assert.NotContains(t, textContent.Text, `"test-agent-1"`)
		assert.Contains(t, textContent.Text, `"agent2"`)
		assert.NotContains(t, textContent.Text, `"test-agent-2"`)
	})

	t.Run("include_agent=true includes detailed agent info", func(t *testing.T) {
		request := createMCPRequest(t, map[string]any{})
		args := GetJobsArgs{
			OrgSlug:      "org",
			PipelineSlug: "pipeline",
			BuildNumber:  "1",
			IncludeAgent: true,
		}
		result, err := handler(ctx, request, args)
		require.NoError(t, err)

		textContent := getTextResult(t, result)
		assert.Contains(t, textContent.Text, `"job1"`)
		assert.Contains(t, textContent.Text, `"job2"`)
		assert.Contains(t, textContent.Text, `"job3"`)
		// Full agent info should be included for jobs that have agents
		assert.Contains(t, textContent.Text, `"agent1"`)
		assert.Contains(t, textContent.Text, `"test-agent-1"`)
		assert.Contains(t, textContent.Text, `"agent2"`)
		assert.Contains(t, textContent.Text, `"test-agent-2"`)
	})
}

func TestGetJobsPaginationWithFilter(t *testing.T) {
	ctx := context.Background()
	client := &MockBuildsClient{
		GetFunc: func(ctx context.Context, org string, pipeline string, id string, opt *buildkite.BuildGetOptions) (buildkite.Build, *buildkite.Response, error) {
			// Create a build with multiple jobs of the same state for filtering
			return buildkite.Build{
					ID:        "123",
					Number:    1,
					State:     "finished",
					CreatedAt: &buildkite.Timestamp{},
					Jobs: []buildkite.Job{
						{ID: "job1", State: "passed", Agent: buildkite.Agent{ID: "agent1", Name: "test-agent-1"}},
						{ID: "job2", State: "failed", Agent: buildkite.Agent{ID: "agent2", Name: "test-agent-2"}},
						{ID: "job3", State: "passed", Agent: buildkite.Agent{ID: "agent3", Name: "test-agent-3"}},
						{ID: "job4", State: "passed", Agent: buildkite.Agent{ID: "agent4", Name: "test-agent-4"}},
						{ID: "job5", State: "passed", Agent: buildkite.Agent{ID: "agent5", Name: "test-agent-5"}},
						{ID: "job6", State: "failed", Agent: buildkite.Agent{ID: "agent6", Name: "test-agent-6"}},
					},
				}, &buildkite.Response{
					Response: &http.Response{
						StatusCode: 200,
					},
				}, nil
		},
	}

	tool, handler, _ := GetJobs(client)
	require.NotNil(t, tool)
	require.NotNil(t, handler)

	// Test pagination with state filter - should have 4 "passed" jobs total
	requestPassedPaginated := createMCPRequest(t, map[string]any{})
	argsPassedPaginated := GetJobsArgs{
		OrgSlug:      "org",
		PipelineSlug: "pipeline",
		BuildNumber:  "1",
		JobState:     "passed",
		Page:         1,
		PerPage:      2,
	}
	resultPassedPaginated, err := handler(ctx, requestPassedPaginated, argsPassedPaginated)
	require.NoError(t, err)

	textContentPassedPaginated := getTextResult(t, resultPassedPaginated)
	// Should contain first 2 "passed" jobs
	assert.Contains(t, textContentPassedPaginated.Text, `"job1"`)
	assert.Contains(t, textContentPassedPaginated.Text, `"job3"`)
	assert.NotContains(t, textContentPassedPaginated.Text, `"job2"`) // failed job
	assert.NotContains(t, textContentPassedPaginated.Text, `"job4"`) // not on this page
	// Should have pagination metadata
	assert.Contains(t, textContentPassedPaginated.Text, `"page":1`)
	assert.Contains(t, textContentPassedPaginated.Text, `"per_page":2`)
	assert.Contains(t, textContentPassedPaginated.Text, `"total":4`)
	assert.Contains(t, textContentPassedPaginated.Text, `"has_next":true`)
	assert.Contains(t, textContentPassedPaginated.Text, `"has_prev":false`)
}

func TestGetJobLogs(t *testing.T) {
	// Test the tool definition
	t.Run("ToolDefinition", func(t *testing.T) {
		tool, _ := GetJobLogs(nil)

		assert.Equal(t, "get_job_logs", tool.Name)
		assert.Contains(t, tool.Description, "Get the log output and metadata for a specific job, including content, size, and header timestamps")
	})

	t.Run("MissingParameters", func(t *testing.T) {
		assert := require.New(t)
		_, handler := GetJobLogs(&buildkite.Client{})

		// Test missing org parameter
		req := createMCPRequest(t, map[string]any{})
		args := GetJobLogsArgs{
			PipelineSlug: "test-pipeline",
			BuildNumber:  "123",
			JobUUID:      "job-123",
		}
		result, err := handler(context.Background(), req, args)
		assert.NoError(err)
		assert.NotNil(result)
		assert.NotEmpty(result.Content)

		// Test missing pipeline_slug parameter
		req = createMCPRequest(t, map[string]any{})
		args = GetJobLogsArgs{
			OrgSlug:     "test-org",
			BuildNumber: "123",
			JobUUID:     "job-123",
		}
		result, err = handler(context.Background(), req, args)
		assert.NoError(err)
		assert.NotNil(result)
		assert.NotEmpty(result.Content)

		// Test missing build_number parameter
		req = createMCPRequest(t, map[string]any{})
		args = GetJobLogsArgs{
			OrgSlug:      "test-org",
			PipelineSlug: "test-pipeline",
			JobUUID:      "job-123",
		}
		result, err = handler(context.Background(), req, args)
		assert.NoError(err)
		assert.NotNil(result)
		assert.NotEmpty(result.Content)

		// Test missing job_uuid parameter
		req = createMCPRequest(t, map[string]any{})
		args = GetJobLogsArgs{
			OrgSlug:      "test-org",
			PipelineSlug: "test-pipeline",
			BuildNumber:  "123",
		}
		result, err = handler(context.Background(), req, args)
		assert.NoError(err)
		assert.NotNil(result)
		assert.NotEmpty(result.Content)
	})
}

// MockJobsClient for testing unblock functionality
type MockJobsClient struct {
	UnblockJobFunc func(ctx context.Context, org string, pipeline string, buildNumber string, jobID string, opt *buildkite.JobUnblockOptions) (buildkite.Job, *buildkite.Response, error)
	GetJobLogFunc  func(ctx context.Context, org string, pipeline string, buildNumber string, jobID string) (buildkite.JobLog, *buildkite.Response, error)
}

func (m *MockJobsClient) UnblockJob(ctx context.Context, org string, pipeline string, buildNumber string, jobID string, opt *buildkite.JobUnblockOptions) (buildkite.Job, *buildkite.Response, error) {
	if m.UnblockJobFunc != nil {
		return m.UnblockJobFunc(ctx, org, pipeline, buildNumber, jobID, opt)
	}
	return buildkite.Job{}, &buildkite.Response{}, nil
}

func (m *MockJobsClient) GetJobLog(ctx context.Context, org string, pipeline string, buildNumber string, jobID string) (buildkite.JobLog, *buildkite.Response, error) {
	if m.GetJobLogFunc != nil {
		return m.GetJobLogFunc(ctx, org, pipeline, buildNumber, jobID)
	}
	return buildkite.JobLog{}, &buildkite.Response{}, nil
}

func TestUnblockJob(t *testing.T) {
	ctx := context.Background()

	// Test tool definition
	t.Run("ToolDefinition", func(t *testing.T) {
		tool, _, _ := UnblockJob(&MockJobsClient{})
		assert.Equal(t, "unblock_job", tool.Name)
		assert.Contains(t, tool.Description, "Unblock a blocked job")
	})

	// Test successful unblock
	t.Run("SuccessfulUnblock", func(t *testing.T) {
		mockJobs := &MockJobsClient{
			UnblockJobFunc: func(ctx context.Context, org string, pipeline string, buildNumber string, jobID string, opt *buildkite.JobUnblockOptions) (buildkite.Job, *buildkite.Response, error) {
				assert.Equal(t, "test-org", org)
				assert.Equal(t, "test-pipeline", pipeline)
				assert.Equal(t, "123", buildNumber)
				assert.Equal(t, "job-123", jobID)

				return buildkite.Job{
						ID:    jobID,
						State: "unblocked",
					}, &buildkite.Response{
						Response: &http.Response{
							StatusCode: 200,
						},
					}, nil
			},
		}

		_, handler, _ := UnblockJob(mockJobs)

		req := createMCPRequest(t, map[string]any{})
		args := UnblockJobArgs{
			OrgSlug:      "test-org",
			PipelineSlug: "test-pipeline",
			BuildNumber:  "123",
			JobID:        "job-123",
		}

		result, err := handler(ctx, req, args)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Contains(t, result.Content[0].(mcp.TextContent).Text, `"id":"job-123"`)
		assert.Contains(t, result.Content[0].(mcp.TextContent).Text, `"state":"unblocked"`)
	})

	// Test with fields
	t.Run("UnblockWithFields", func(t *testing.T) {
		mockJobs := &MockJobsClient{
			UnblockJobFunc: func(ctx context.Context, org string, pipeline string, buildNumber string, jobID string, opt *buildkite.JobUnblockOptions) (buildkite.Job, *buildkite.Response, error) {
				// Verify fields were passed correctly
				require.NotNil(t, opt)
				assert.Equal(t, "v1.0.0", opt.Fields["version"])
				assert.Equal(t, "prod", opt.Fields["environment"])

				return buildkite.Job{
						ID:    jobID,
						State: "unblocked",
					}, &buildkite.Response{
						Response: &http.Response{
							StatusCode: 200,
						},
					}, nil
			},
		}

		_, handler, _ := UnblockJob(mockJobs)

		req := createMCPRequest(t, map[string]any{})
		args := UnblockJobArgs{
			OrgSlug:      "test-org",
			PipelineSlug: "test-pipeline",
			BuildNumber:  "123",
			JobID:        "job-123",
			Fields:       map[string]string{"version": "v1.0.0", "environment": "prod"},
		}

		result, err := handler(ctx, req, args)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	// Test client error
	t.Run("ClientError", func(t *testing.T) {
		mockJobs := &MockJobsClient{
			UnblockJobFunc: func(ctx context.Context, org string, pipeline string, buildNumber string, jobID string, opt *buildkite.JobUnblockOptions) (buildkite.Job, *buildkite.Response, error) {
				return buildkite.Job{}, nil, errors.New("API connection failed")
			},
		}

		_, handler, _ := UnblockJob(mockJobs)

		req := createMCPRequest(t, map[string]any{})
		args := UnblockJobArgs{
			OrgSlug:      "test-org",
			PipelineSlug: "test-pipeline",
			BuildNumber:  "123",
			JobID:        "job-123",
		}

		result, err := handler(ctx, req, args)
		require.NoError(t, err)
		assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "API connection failed")
	})

	// Test missing parameters
	t.Run("MissingParameters", func(t *testing.T) {
		_, handler, _ := UnblockJob(&MockJobsClient{})

		// Test missing org parameter
		req := createMCPRequest(t, map[string]any{})
		args := UnblockJobArgs{
			PipelineSlug: "test-pipeline",
			BuildNumber:  "123",
			JobID:        "job-123",
		}
		result, err := handler(ctx, req, args)
		require.NoError(t, err)
		assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "org_slug parameter is required")

		// Test missing pipeline_slug parameter
		args = UnblockJobArgs{
			OrgSlug:     "test-org",
			BuildNumber: "123",
			JobID:       "job-123",
		}
		result, err = handler(ctx, req, args)
		require.NoError(t, err)
		assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "pipeline_slug parameter is required")

		// Test missing build_number parameter
		args = UnblockJobArgs{
			OrgSlug:      "test-org",
			PipelineSlug: "test-pipeline",
			JobID:        "job-123",
		}
		result, err = handler(ctx, req, args)
		require.NoError(t, err)
		assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "build_number parameter is required")

		// Test missing job_id parameter
		args = UnblockJobArgs{
			OrgSlug:      "test-org",
			PipelineSlug: "test-pipeline",
			BuildNumber:  "123",
		}
		result, err = handler(ctx, req, args)
		require.NoError(t, err)
		assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "job_id parameter is required")
	})
}
