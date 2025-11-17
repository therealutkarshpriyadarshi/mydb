# Documentation Guidelines for StoreOur Project

## Purpose

This document defines the standard for creating comprehensive README files for packages in the StoreOur database system. Following these guidelines ensures that documentation is accessible and helpful for first-time readers.

---

## When to Create Package Documentation

Create a README.md file for any package that:
- Contains core abstractions or interfaces
- Implements complex algorithms or patterns
- Will be used by other developers
- Has non-obvious usage patterns
- Requires understanding of thread safety or performance characteristics

---

## Required Sections

### 1. **Overview** (Required)

A 2-4 paragraph summary explaining:
- What the package does
- Why it exists
- What problems it solves
- High-level capabilities

**Example:**
```markdown
## Overview

This package implements a flexible aggregation system that separates the
aggregation logic from the calculation logic. This separation makes it easy
to add new aggregate functions without modifying the core iteration logic.
```

---

### 2. **Table of Contents** (Required for README > 500 lines)

Link to all major sections for easy navigation.

---

### 3. **Key Concepts** (Required)

Break down the main abstractions/concepts into digestible pieces:
- Use numbered or bulleted subsections
- Explain each concept in 2-3 sentences
- Use analogies or metaphors when helpful (e.g., "Think of it as the orchestrator")

**Example:**
```markdown
## Key Concepts

### 1. **Aggregator**
The orchestrator that handles the workflow.

### 2. **Calculator**
The specialist that knows how to compute specific functions.
```

---

### 4. **Components** (Required)

List all major types, interfaces, and functions with:
- Purpose/responsibility
- Location (file reference)
- Key methods (for types/interfaces)

Use tables or structured lists.

**Example:**
```markdown
## Components

### Core Interfaces

#### `GroupAggregator`
Provides methods to get all group keys and retrieve aggregate values.
**Location**: [interfaces.go](./interfaces.go)
```

---

### 5. **Getting Started** (Required)

Include:
- Prerequisites
- Basic workflow steps (numbered list)
- What you need before using the package

**Example:**
```markdown
## Getting Started

### Basic Workflow
1. Create a calculator for your aggregate type
2. Create a BaseAggregator with the calculator
3. Feed tuples using Merge()
4. Iterate results
```

---

### 6. **Usage Examples** (Required - Minimum 3)

Provide at least 3 real, runnable code examples:
- Start with simple use case
- Progress to more complex scenarios
- Include a custom implementation example if applicable
- Add explanatory comments
- Show both setup and usage

**Example:**
```markdown
### Example 1: Simple COUNT(*) Query

```go
// SELECT COUNT(*) FROM orders
calculator := NewIntCalculator(OpCount)
agg, err := NewBaseAggregator(...)
...
```
```

---

### 7. **Thread Safety** (Required if applicable)

Document:
- What's protected and how
- Usage guidelines
- Safe concurrent patterns
- Example of correct concurrent usage

**Example:**
```markdown
## Thread Safety

### What's Protected?
- `Merge()` is thread-safe
- Each goroutine should have its own iterator

### Safe Pattern
```go
go func() { agg.Merge(tuple) }()  // Safe
```
```

---

### 8. **Architecture** (Recommended)

Include:
- Data flow diagrams (ASCII art or descriptions)
- Result format specifications
- Behavioral notes (e.g., snapshot behavior)

**Example:**
```markdown
## Architecture

### Data Flow
```
Input Tuples → Merge() → Calculator → Iterator → Output
```

### Result Tuple Format
Non-grouped: [aggregateValue]
Grouped: [groupKey, aggregateValue]
```

---

### 9. **File Reference** (Recommended)

Table mapping files to their purposes.

**Example:**
```markdown
| File | Purpose |
|------|---------|
| base.go | BaseAggregator implementation |
| interfaces.go | Core interfaces |
```

---

### 10. **Testing** (Recommended)

Provide commands for:
- Running all tests
- Running with coverage
- Running specific tests

**Example:**
```markdown
## Testing

Run tests:
```bash
go test ./pkg/path
```
```

---

### 11. **Common Pitfalls** (Highly Recommended)

Show wrong ❌ and correct ✅ examples for common mistakes:

**Example:**
```markdown
## Common Pitfalls

### 1. Not Opening Iterator Before Use

```go
// ❌ Wrong
iter := agg.Iterator()
tuple, err := iter.Next()  // Error!

// ✅ Correct
iter := agg.Iterator()
iter.Open()
tuple, err := iter.Next()  // Works!
```
```

---

### 12. **Performance Considerations** (Optional but recommended)

Document:
- Memory usage patterns (Big-O)
- Optimization tips
- Trade-offs

---

### 13. **Future Enhancements** (Optional)

List potential improvements as checkboxes:

**Example:**
```markdown
## Future Enhancements

- [ ] Support for DISTINCT aggregates
- [ ] Parallel aggregation for large datasets
```

---

### 14. **Related Packages** (Recommended)

List dependencies and related packages with brief descriptions.

---

### 15. **Questions/Getting Help** (Optional)

Guide readers on where to go for help:

**Example:**
```markdown
## Questions?

1. Start with the examples
2. Read the interfaces
3. Check the tests
```

---

## Formatting Standards

### Code Blocks

Always specify the language:
```markdown
```go
func example() {}
```
```

### Emojis

Use sparingly for visual markers:
- ✅ Correct example
- ❌ Wrong example
- 🚀 End of guide/encouragement
- ⚠️ Warning/caution

### Headings

- Use `##` for major sections
- Use `###` for subsections
- Use `####` for sub-subsections

### Lists

- Use numbered lists for sequential steps
- Use bullet points for unordered items
- Use checkboxes `- [ ]` for todo/feature lists

### Visual Separators

Use `---` (horizontal rules) between major sections.

### Links

Link to actual files:
```markdown
**Location**: [interfaces.go](./interfaces.go)
```

---

## Calling This Guideline from Claude

To invoke these guidelines when working with Claude Code, use one of these approaches:

### Approach 1: Direct Reference
```
Create a README for the XYZ package following the documentation guidelines in .claude/documentation-guidelines.md
```

### Approach 2: Shorthand
```
Create a comprehensive README for the XYZ package with all standard sections: overview, key concepts, components, getting started, 3+ examples, thread safety, architecture, common pitfalls, etc.
```

### Approach 3: Hook Integration

Add this to your `.claude/settings.local.json` under `hooks`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "create.*readme|document.*package",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Reminder: Follow .claude/documentation-guidelines.md for comprehensive documentation'"
          }
        ]
      }
    ]
  }
}
```

This will show a reminder whenever you ask Claude to create documentation.

---

## Examples of Good Documentation

Reference these package READMEs as examples:
- [pkg/execution/aggregation/internal/core/README.md](../../pkg/execution/aggregation/internal/core/README.md)

---

## Checklist for Review

Before finalizing documentation, verify:

- [ ] Overview explains purpose clearly
- [ ] Key concepts are broken down into digestible pieces
- [ ] All major types/interfaces are documented
- [ ] At least 3 usage examples are provided
- [ ] Thread safety is addressed (if applicable)
- [ ] Common pitfalls section with ❌/✅ examples
- [ ] Code examples are syntactically correct
- [ ] File references link to actual files
- [ ] Testing instructions are provided
- [ ] Visual separators improve readability
- [ ] TOC included (if README > 500 lines)

---

**Last Updated**: 2025-10-25
