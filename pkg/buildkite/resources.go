package buildkite

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func HandleDebugLogsGuideResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	content := `# Debugging Buildkite Build Failures with Job Logs

This guide explains how to effectively use the Buildkite MCP server's job log tools to debug build failures.

## Table of Contents
- [Tools Overview](#tools-overview)
- [Debugging Workflow](#debugging-workflow)
- [Optimizing LLM Usage](#optimizing-llm-usage)
- [Common Error Patterns](#common-error-patterns)
- [Example Investigation](#example-investigation)
- [LLM Prompt Templates](#llm-prompt-templates)

## Tools Overview

The server provides four powerful tools for log analysis:

### 1. get_logs_info - Start Here
**Always begin your investigation with this tool** to understand the job log file size and scope.

` + "```json" + `
{
  "org": "<ORG>",
  "pipeline": "<PIPELINE>", 
  "build": "<BUILD>",
  "job": "<JOB_ID>"
}
` + "```" + `

This helps you plan your debugging approach based on job log size.

### 2. tail_logs - For Recent Failures
**Best for finding recent errors** - shows the last N job log entries where failures typically appear.

` + "```json" + `
{
  "org": "<ORG>",
  "pipeline": "<PIPELINE>",
  "build": "<BUILD>", 
  "job": "<JOB_ID>",
  "tail": 50
}
` + "```" + `

### 3. search_logs - For Specific Issues
**Most powerful tool** for finding specific error patterns with context.

**Key Parameters:**
- ` + "`pattern`" + ` (required): Regex pattern (POSIX-style, case-insensitive by default)
- ` + "`context`" + `: Lines before/after each match (0-20 recommended)
- ` + "`before_context`" + `/` + "`after_context`" + `: Asymmetric context
- ` + "`case_sensitive`" + `: Enable case-sensitive matching
- ` + "`invert_match`" + `: Show non-matching lines
- ` + "`reverse`" + `: Search backwards from end
- ` + "`seek_start`" + `: Start from specific row (0-based)
- ` + "`limit`" + `: Max matches (default: 100)

` + "```json" + `
{
  "org": "<ORG>",
  "pipeline": "<PIPELINE>",
  "build": "<BUILD>",
  "job": "<JOB_ID>", 
  "pattern": "error|failed|exception",
  "context": 3,
  "limit": 20
}
` + "```" + `

> ⚠️ **Warning**: Setting ` + "`limit`" + ` > 200 may exceed LLM context windows.

### 4. read_logs - For Sequential Reading
**Use when you need to read specific sections** of logs in order.

` + "```json" + `
{
  "org": "<ORG>",
  "pipeline": "<PIPELINE>",
  "build": "<BUILD>",
  "job": "<JOB_ID>",
  "seek": 1000,
  "limit": 100
}
` + "```" + `

## Debugging Workflow

### Step 1: Quick Assessment
1. Start with ` + "`get_logs_info`" + ` to understand log size
2. Use ` + "`tail_logs`" + ` with ` + "`tail: 50-100`" + ` to see recent entries

### Step 2: Error Hunting
3. Use ` + "`search_logs`" + ` with common error patterns:
   - ` + "`error|failed|exception`" + `
   - ` + "`fatal|panic|abort`" + `
   - ` + "`timeout|cancelled`" + `
   - ` + "`permission denied|access denied`" + `

### Step 3: Context Investigation
4. When you find errors, increase ` + "`context: 5-10`" + ` to see surrounding lines
5. Use ` + "`before_context`" + ` and ` + "`after_context`" + ` for asymmetric context

### Step 4: Deep Dive
6. Use ` + "`read_logs`" + ` with ` + "`seek`" + ` to read specific sections around errors
7. Search for test names, file paths, or specific commands that failed

## Sample Response Format

The ` + "`json-terse`" + ` format returns entries like:
` + "```json" + `
{"ts": 1696168225123, "c": "Test failed: assertion error", "rn": 42}
{"ts": 1696168225456, "c": "npm test", "rn": 43}
` + "```" + `
- ` + "`ts`" + `: Timestamp in Unix milliseconds
- ` + "`c`" + `: Log content (ANSI codes stripped)
- ` + "`rn`" + `: Row number (0-based, use for seeking)

## Optimizing LLM Usage

### Token Efficiency
- **Always use ` + "`format: \"json-terse\"`" + `** (default) for most efficient token usage
  - Provides both log content (` + "`c`" + `) and row numbers (` + "`rn`" + `) for precise pagination
  - Automatically strips ANSI escape codes for clean processing
  - Most compact representation for AI analysis
- **Always set ` + "`limit`" + ` parameters** to avoid excessive output
- Use ` + "`raw: true`" + ` when you only need log content without metadata

### Progressive Search Strategy
1. Start broad with low limits (` + "`limit: 10-20`" + `)
2. Refine patterns based on findings
3. Use ` + "`invert_match: true`" + ` to exclude noise
4. Use ` + "`reverse: true`" + ` to search backwards from known failure points

### Common Error Patterns

**Build failures:**
` + "```" + `
"pattern": "build failed|compilation error|linking error"
` + "```" + `

**Test failures:**
` + "```" + `
"pattern": "test.*failed|assertion.*failed|expected.*but got"
` + "```" + `

**Infrastructure issues:**
` + "```" + `
"pattern": "network.*error|timeout|connection.*refused|dns.*error"
` + "```" + `

**Permission/security:**
` + "```" + `
"pattern": "permission denied|access denied|unauthorized|forbidden"
` + "```" + `

### Context Guidelines
- Use ` + "`context: 3-5`" + ` for general investigation
- Use ` + "`context: 10-20`" + ` when you need to understand complex error flows
- Limit context to avoid token waste on unrelated log entries

### JSON-Terse Format Benefits
The ` + "`json-terse`" + ` format is specifically designed for efficient AI processing:
- **Row Numbers**: ` + "`rn`" + ` field enables precise seeking with ` + "`read_logs`" + ` for context around found issues
- **Clean Content**: Automatically strips ANSI escape codes that would waste tokens
- **Compact Structure**: Minimal field names (` + "`ts`" + `, ` + "`c`" + `, ` + "`rn`" + `) reduce overhead
- **Pagination Support**: Use row numbers to fetch precise context around errors

## Example Investigation

` + "```json" + `
// 1. Get file overview
{"org": "<ORG>", "pipeline": "<PIPELINE>", "build": "<BUILD>", "job": "<JOB_ID>"}

// 2. Check recent failures  
{"org": "<ORG>", "pipeline": "<PIPELINE>", "build": "<BUILD>", "job": "<JOB_ID>", "tail": 50}

// 3. Search for errors with context
{"org": "<ORG>", "pipeline": "<PIPELINE>", "build": "<BUILD>", "job": "<JOB_ID>", "pattern": "failed|error", "context": 5, "limit": 15}

// 4. Deep dive on specific test failures
{"org": "<ORG>", "pipeline": "<PIPELINE>", "build": "<BUILD>", "job": "<JOB_ID>", "pattern": "TestLoginHandler.*failed", "context": 10, "limit": 5}
` + "```" + `

## Cache Management

- Completed builds are cached permanently
- Running builds use 30s TTL by default
- Use ` + "`force_refresh: true`" + ` only when you need the absolute latest data
- Set ` + "`cache_ttl`" + ` appropriately for your investigation needs

## LLM Prompt Templates

For AI assistants debugging build failures:

` + "```" + `
Standard debugging workflow:
1. Call get_logs_info to assess log size
2. If rows > 1000: use tail_logs with tail=50 first
3. Search with pattern "(error|failed|timeout)" limit=15 context=3
4. For each critical error: use read_logs with seek=<rn-10> limit=20
5. Summarize findings in 3 bullet points max
` + "```" + `

**Token estimation guide:**
- ` + "`get_logs_info`" + `: ~50 tokens
- ` + "`tail_logs`" + ` (50 lines): ~800-1200 tokens  
- ` + "`search_logs`" + ` (20 matches): ~1000-2000 tokens
- ` + "`read_logs`" + ` (100 lines): ~1500-2500 tokens

> 💡 **Tip**: After collecting log data, summarize key findings to reduce context for follow-up queries.

This systematic approach will help you quickly identify and understand build failures using the available job log tools.
`

	return []mcp.ResourceContents{
		&mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/markdown",
			Text:     content,
		},
	}, nil
}
