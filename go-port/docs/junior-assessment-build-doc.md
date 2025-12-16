# Junior Assessment Feature - Build Document

## Overview

Add a `/junior-assessment` command that has Senior evaluate the currently loaded Junior model through a series of coding challenges, then documents the results in `CLAUDE.md`.

## Purpose

- Give Senior knowledge of Junior's capabilities before delegating work
- Create a persistent record of model assessments
- Help users understand what tasks are safe to delegate

## User Flow

```
You> /junior-assessment

Senior> Starting Junior assessment...

[Senior runs 10-20 coding challenges against Junior]
[Senior evaluates responses and scores them]
[Senior writes summary to CLAUDE.md]

Senior> Assessment complete. Results saved to CLAUDE.md

  Model: gpt-oss-20b-fp16
  Score: 72/80 (90%)
  Strengths: Data structures, algorithms, docstrings
  Weaknesses: Complex async patterns

  See CLAUDE.md for full details.
```

## Technical Design

### New Files

```
go-port/
├── internal/
│   └── assessment/
│       ├── assessment.go      # Core assessment logic
│       ├── challenges.go      # Coding challenge definitions
│       ├── scorer.go          # Response evaluation
│       └── report.go          # CLAUDE.md generation
```

### Command Handler (cmd/weaver/main.go)

```go
case "/junior-assessment":
    fmt.Println("\nStarting Junior assessment...")
    report, err := weaver.AssessJunior(ctx)
    if err != nil {
        fmt.Printf(colorRed+"Error: %v"+colorReset+"\n", err)
        return true
    }
    // Write to CLAUDE.md
    err = writeAssessmentReport(report)
    if err != nil {
        fmt.Printf(colorRed+"Failed to save report: %v"+colorReset+"\n", err)
    }
    fmt.Println("\nAssessment complete. Results saved to CLAUDE.md")
    printAssessmentSummary(report)
    return true
```

### Assessment Structure

```go
type AssessmentReport struct {
    Timestamp      time.Time
    ModelName      string
    Endpoint       string
    TotalScore     int
    MaxScore       int
    Percentage     float64
    Strengths      []string
    Weaknesses     []string
    RecommendTasks []string
    AvoidTasks     []string
    Details        []ChallengeResult
}

type ChallengeResult struct {
    Category    string  // "algorithms", "data_structures", "code_quality", etc.
    Name        string  // "FizzBuzz", "Two Sum", etc.
    Score       int     // 0-4
    MaxScore    int     // 4
    Notes       string  // Senior's evaluation notes
}
```

### Challenge Categories (10-20 challenges)

| Category | Count | Examples |
|----------|-------|----------|
| Basic Algorithms | 3 | FizzBuzz, Palindrome, Fibonacci |
| Data Structures | 3 | Two Sum, Group By, Linked List |
| Code Quality | 3 | Docstrings, Type hints, Error handling |
| Real-World | 3 | File parser, Config reader, HTTP client |
| Problem Solving | 3 | Edge cases, Bug identification, Refactoring |

### Scoring Rubric

- **0** - Failed/incorrect
- **1** - Works but poor quality
- **2** - Correct with minor issues
- **3** - Correct and clean
- **4** - Excellent (exceeds requirements)

### CLAUDE.md Output Format

```markdown
## Junior Engineer Assessment

**Last Assessment:** 2025-12-06 14:30:00
**Model:** gpt-oss-20b-fp16
**Endpoint:** http://localhost:1234/v1

### Results

| Category | Score | Percentage |
|----------|-------|------------|
| Basic Algorithms | 11/12 | 92% |
| Data Structures | 10/12 | 83% |
| Code Quality | 12/12 | 100% |
| Real-World | 9/12 | 75% |
| Problem Solving | 10/12 | 83% |
| **Total** | **52/60** | **87%** |

### Strengths
- Excellent code documentation and type hints
- Strong algorithm implementation
- Good understanding of data structures

### Weaknesses
- Struggles with complex async patterns
- May miss edge cases in real-world scenarios

### Delegation Guidelines

**Recommended Tasks:**
- Boilerplate code generation
- Unit test writing
- Documentation and docstrings
- Simple refactoring
- Data structure implementations

**Avoid Delegating:**
- Security-critical code
- Complex async/concurrent logic
- Architecture decisions
- Production error handling
```

## Implementation Steps

1. [ ] Create `internal/assessment/` package structure
2. [ ] Define challenge set in `challenges.go`
3. [ ] Implement assessment runner in `assessment.go`
4. [ ] Have Senior evaluate responses (via delegation back to Senior)
5. [ ] Generate report in `report.go`
6. [ ] Add `/junior-assessment` command to CLI
7. [ ] Write CLAUDE.md output
8. [ ] Add tests
9. [ ] Update help text

## Key Decisions

### Who Evaluates Junior's Responses?

**Option A:** Hardcoded rubric (pattern matching, test execution)
- Pros: Fast, deterministic
- Cons: Can't evaluate code quality nuances

**Option B:** Senior evaluates Junior (recommended)
- Pros: Nuanced evaluation, natural language feedback
- Cons: Slower, uses API calls

**Decision:** Option B - Senior evaluates. This tests the full delegation flow and gives better quality assessments.

### Where to Store Results?

**Option A:** CLAUDE.md in current directory
**Option B:** ~/.weaver/assessments/
**Option C:** Both (local + global history)

**Decision:** Option A for now - CLAUDE.md in current working directory. Simple and visible.

### How Many Challenges?

- Quick: 5 challenges (~2 min)
- Standard: 10 challenges (~5 min) ← Default
- Thorough: 20 challenges (~15 min)

Could add `--quick` / `--thorough` flags later.

## Dependencies

- No new external dependencies
- Uses existing orchestrator for Senior/Junior communication
- Uses existing file I/O patterns

## Testing

```bash
# Unit tests
go test ./internal/assessment/...

# Integration test (requires Junior model running)
./weaver
> /junior-assessment
```

## Future Enhancements (Not in Scope)

- [ ] Assessment history tracking
- [ ] Model comparison reports
- [ ] Custom challenge sets
- [ ] Export to JSON/YAML
- [ ] Integration with /setup wizard
