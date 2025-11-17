package planner

import (
	"fmt"
	"storemy/pkg/plan"
	"strings"
)

// formatPlanEducational formats the plan with educational explanations.
// This helps users understand what the database is doing and why.
func (p *ExplainPlan) formatPlanEducational(planNode plan.PlanNode) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("┌" + strings.Repeat("─", 78) + "┐\n")
	sb.WriteString("│" + center("🎓 EDUCATIONAL QUERY PLAN EXPLANATION", 78) + "│\n")
	sb.WriteString("├" + strings.Repeat("─", 78) + "┤\n")
	sb.WriteString("│ This shows how your database will execute the query step-by-step          │\n")
	sb.WriteString("└" + strings.Repeat("─", 78) + "┘\n\n")

	// Add execution order explanation
	sb.WriteString("📋 EXECUTION ORDER (bottom-up):\n")
	sb.WriteString(strings.Repeat("─", 80))
	sb.WriteString("\n\n")

	// Format the plan tree with educational annotations
	sb.WriteString(p.formatPlanEducationalRecursive(planNode, 0, 1))

	// Add summary section
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("═", 80))
	sb.WriteString("\n")
	sb.WriteString("📊 PERFORMANCE SUMMARY\n")
	sb.WriteString(strings.Repeat("─", 80))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Total Estimated Cost: %.2f units\n", planNode.GetCost()))
	sb.WriteString(fmt.Sprintf("  Estimated Rows:       %d rows\n", planNode.GetCardinality()))
	sb.WriteString("\n")

	// Add performance tips
	sb.WriteString(p.generatePerformanceTips(planNode))

	// Add educational resources
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("═", 80))
	sb.WriteString("\n")
	sb.WriteString("💡 LEARN MORE\n")
	sb.WriteString(strings.Repeat("─", 80))
	sb.WriteString("\n")
	sb.WriteString(p.getEducationalResources(planNode))

	return sb.String()
}

// formatPlanEducationalRecursive recursively formats the plan with educational annotations
func (p *ExplainPlan) formatPlanEducationalRecursive(node plan.PlanNode, depth int, step int) string {
	if node == nil {
		return ""
	}

	var sb strings.Builder
	prefix := strings.Repeat("  ", depth)

	// Format this node with educational explanation
	sb.WriteString(fmt.Sprintf("%sStep %d: %s\n", prefix, step, p.formatNodeEducational(node)))
	sb.WriteString(fmt.Sprintf("%s        %s\n", prefix, p.getNodeExplanation(node)))
	sb.WriteString(fmt.Sprintf("%s        Cost: %.2f | Rows: %d\n\n", prefix, node.GetCost(), node.GetCardinality()))

	// Recursively format children (process them in reverse to show bottom-up execution)
	children := node.GetChildren()
	currentStep := step + 1
	for i := len(children) - 1; i >= 0; i-- {
		if children[i] != nil {
			sb.WriteString(p.formatPlanEducationalRecursive(children[i], depth+1, currentStep))
			currentStep += p.countNodes(children[i])
		}
	}

	return sb.String()
}

// getNodeExplanation returns a user-friendly explanation of what this node does
func (p *ExplainPlan) getNodeExplanation(node plan.PlanNode) string {
	switch n := node.(type) {
	case *plan.ScanNode:
		if n.AccessMethod == "indexscan" {
			return fmt.Sprintf("💡 Using index '%s' to quickly find matching rows", n.IndexName)
		}
		return "💡 Reading all rows from table (full table scan)"

	case *plan.JoinNode:
		return fmt.Sprintf("💡 Combining data from two tables using %s method", n.JoinMethod)

	case *plan.FilterNode:
		return fmt.Sprintf("💡 Filtering out rows that don't match %d condition(s)", len(n.Predicates))

	case *plan.ProjectNode:
		return fmt.Sprintf("💡 Selecting only %d specific column(s) from the result", len(n.Columns))

	case *plan.AggregateNode:
		if len(n.GroupByExprs) > 0 {
			return fmt.Sprintf("💡 Grouping rows by '%s' and computing aggregate(s)", strings.Join(n.GroupByExprs, ", "))
		}
		return "💡 Computing aggregate function(s) over all rows"

	case *plan.SortNode:
		order := "ascending"
		if !n.Ascending {
			order = "descending"
		}
		return fmt.Sprintf("💡 Sorting rows by '%s' in %s order", n.SortKey, order)

	case *plan.LimitNode:
		if n.Offset > 0 {
			return fmt.Sprintf("💡 Skipping first %d rows, then returning up to %d rows", n.Offset, n.Limit)
		}
		return fmt.Sprintf("💡 Returning only the first %d rows", n.Limit)

	case *plan.DistinctNode:
		return "💡 Removing duplicate rows from the result"

	case *plan.SetOpNode:
		return fmt.Sprintf("💡 Combining results using %s operation", n.OpType)

	case *plan.InsertNode:
		return fmt.Sprintf("💡 Inserting %d row(s) into the table", n.NumRows)

	case *plan.UpdateNode:
		return fmt.Sprintf("💡 Updating %d field(s) in matching rows", n.SetFields)

	case *plan.DeleteNode:
		return "💡 Deleting rows that match the criteria"

	case *plan.DDLNode:
		return fmt.Sprintf("💡 Schema operation: %s", n.Operation)

	default:
		return "💡 Processing data"
	}
}

// formatNodeEducational formats a single node with user-friendly description
func (p *ExplainPlan) formatNodeEducational(node plan.PlanNode) string {
	switch n := node.(type) {
	case *plan.ScanNode:
		method := "Sequential Scan"
		if n.AccessMethod == "indexscan" {
			method = "Index Scan"
		}
		return fmt.Sprintf("%s on table '%s'", method, n.TableName)

	case *plan.JoinNode:
		return fmt.Sprintf("%s Join (%s = %s)", n.JoinType, n.LeftColumn, n.RightColumn)

	case *plan.FilterNode:
		return "Apply Filter"

	case *plan.ProjectNode:
		return "Select Columns"

	case *plan.AggregateNode:
		return fmt.Sprintf("Aggregate [%s]", strings.Join(n.AggFunctions, ", "))

	case *plan.SortNode:
		return fmt.Sprintf("Sort by %s", n.SortKey)

	case *plan.LimitNode:
		return "Limit Results"

	case *plan.DistinctNode:
		return "Remove Duplicates"

	case *plan.SetOpNode:
		return n.OpType

	case *plan.InsertNode:
		return fmt.Sprintf("Insert into '%s'", n.TableName)

	case *plan.UpdateNode:
		return fmt.Sprintf("Update table '%s'", n.TableName)

	case *plan.DeleteNode:
		return fmt.Sprintf("Delete from '%s'", n.TableName)

	case *plan.DDLNode:
		return fmt.Sprintf("%s '%s'", n.Operation, n.ObjectName)

	default:
		return node.GetNodeType()
	}
}

// generatePerformanceTips generates helpful performance tips based on the plan
func (p *ExplainPlan) generatePerformanceTips(node plan.PlanNode) string {
	var tips []string

	// Analyze the plan tree for optimization opportunities
	p.collectPerformanceTips(node, &tips)

	if len(tips) == 0 {
		return "  ✅ Your query looks well-optimized!\n"
	}

	var sb strings.Builder
	sb.WriteString("⚡ PERFORMANCE TIPS\n")
	sb.WriteString(strings.Repeat("─", 80))
	sb.WriteString("\n")

	for i, tip := range tips {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, tip))
	}

	return sb.String()
}

// collectPerformanceTips recursively collects performance tips
func (p *ExplainPlan) collectPerformanceTips(node plan.PlanNode, tips *[]string) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *plan.ScanNode:
		if n.AccessMethod == "seqscan" && len(n.Predicates) > 0 {
			*tips = append(*tips, fmt.Sprintf(
				"Consider creating an index on table '%s' for column(s) in WHERE clause to speed up filtering",
				n.TableName,
			))
		}

	case *plan.JoinNode:
		if n.JoinMethod == "" || n.JoinMethod == "nested-loop" {
			*tips = append(*tips, "Consider creating indexes on join columns for better join performance")
		}

	case *plan.SortNode:
		if node.GetCardinality() > 10000 {
			*tips = append(*tips, "Sorting large result sets can be slow. Consider using LIMIT or adding an index on the sort column")
		}

	case *plan.AggregateNode:
		if len(n.GroupByExprs) > 0 {
			*tips = append(*tips, "GROUP BY operations can benefit from indexes on the grouped columns")
		}
	}

	// Recursively check children
	for _, child := range node.GetChildren() {
		p.collectPerformanceTips(child, tips)
	}
}

// getEducationalResources provides links and explanations for learning more
func (p *ExplainPlan) getEducationalResources(node plan.PlanNode) string {
	var sb strings.Builder

	// Detect what concepts are used in this query
	concepts := make(map[string]string)

	p.collectConcepts(node, concepts)

	sb.WriteString("  Key concepts used in this query:\n\n")

	if len(concepts) == 0 {
		sb.WriteString("  • Basic SELECT query\n")
	} else {
		for concept, desc := range concepts {
			sb.WriteString(fmt.Sprintf("  • %s: %s\n", concept, desc))
		}
	}

	sb.WriteString("\n  📚 To learn more about query optimization:\n")
	sb.WriteString("     - Use EXPLAIN ANALYZE to see actual execution statistics\n")
	sb.WriteString("     - Compare costs between different query formulations\n")
	sb.WriteString("     - Monitor for sequential scans on large tables\n")
	sb.WriteString("     - Create indexes for frequently filtered/joined columns\n")

	return sb.String()
}

// collectConcepts identifies database concepts used in the query
func (p *ExplainPlan) collectConcepts(node plan.PlanNode, concepts map[string]string) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *plan.ScanNode:
		if n.AccessMethod == "indexscan" {
			concepts["Index Scan"] = "Using an index to quickly locate rows"
		} else {
			concepts["Sequential Scan"] = "Reading all rows from a table"
		}

	case *plan.JoinNode:
		concepts["JOIN"] = "Combining rows from two or more tables based on a related column"

	case *plan.AggregateNode:
		concepts["Aggregation"] = "Computing summary values (COUNT, SUM, AVG, etc.)"
		if len(n.GroupByExprs) > 0 {
			concepts["GROUP BY"] = "Grouping rows that have the same values"
		}

	case *plan.SortNode:
		concepts["ORDER BY"] = "Sorting query results"

	case *plan.DistinctNode:
		concepts["DISTINCT"] = "Removing duplicate rows from results"

	case *plan.SetOpNode:
		concepts["Set Operations"] = "Combining results from multiple queries (UNION, INTERSECT, EXCEPT)"
	}

	// Recursively collect from children
	for _, child := range node.GetChildren() {
		p.collectConcepts(child, concepts)
	}
}

// countNodes counts the total number of nodes in the subtree
func (p *ExplainPlan) countNodes(node plan.PlanNode) int {
	if node == nil {
		return 0
	}

	count := 1
	for _, child := range node.GetChildren() {
		count += p.countNodes(child)
	}
	return count
}

// center centers text in a given width
func center(text string, width int) string {
	if len(text) >= width {
		return text
	}
	leftPad := (width - len(text)) / 2
	rightPad := width - len(text) - leftPad
	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}
