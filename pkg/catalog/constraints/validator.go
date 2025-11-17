package constraints

import (
	"fmt"
	"storemy/pkg/catalog/operations"
	"storemy/pkg/catalog/schema"
	"storemy/pkg/catalog/systemtable"
	dberror "storemy/pkg/error"
	"storemy/pkg/primitives"
	"storemy/pkg/tuple"
	"storemy/pkg/types"
	"strings"
)

// IndexSearcher defines the interface for searching indexes.
// This allows the validator to check for duplicate values in UNIQUE constraints.
type IndexSearcher interface {
	// SearchIndexForKey searches an index for a specific key value
	SearchIndexForKey(tx operations.TxContext, tableID primitives.FileID, columnIndex primitives.ColumnID, keyValue types.Field) ([]*tuple.TupleRecordID, error)
}

// Validator handles constraint validation for DML operations.
type Validator struct {
	constraintOps *operations.ConstraintOperations
	columnOps     *operations.ColumnOperations
	indexOps      *operations.IndexOperations
	indexSearcher IndexSearcher
}

// NewValidator creates a new constraint validator.
//
// Parameters:
//   - constraintOps: Operations for accessing constraint metadata
//   - columnOps: Operations for accessing column metadata
//   - indexOps: Operations for accessing index metadata
//   - indexSearcher: Interface for searching indexes (can be nil, disables UNIQUE validation)
//
// Returns a new Validator instance.
func NewValidator(
	constraintOps *operations.ConstraintOperations,
	columnOps *operations.ColumnOperations,
	indexOps *operations.IndexOperations,
	indexSearcher IndexSearcher,
) *Validator {
	return &Validator{
		constraintOps: constraintOps,
		columnOps:     columnOps,
		indexOps:      indexOps,
		indexSearcher: indexSearcher,
	}
}

// ValidateInsert validates a tuple against all enabled constraints before insertion.
//
// Validates:
//   - NOT NULL constraints
//   - CHECK constraints (if implemented)
//   - PRIMARY KEY uniqueness (requires index check - not implemented here)
//   - UNIQUE constraints (requires index check - not implemented here)
//   - FOREIGN KEY constraints (requires existence check - not implemented here)
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table being inserted into
//   - tableName: Name of the table (for error messages)
//   - tup: The tuple to validate
//   - sch: The table schema
//
// Returns a DBError if validation fails, nil otherwise.
func (v *Validator) ValidateInsert(tx operations.TxContext, tableID primitives.FileID, tableName string, tup *tuple.Tuple, sch *schema.Schema) error {
	// Get all enabled constraints for the table
	constraints, err := v.constraintOps.GetEnabledConstraintsForTable(tx, tableID)
	if err != nil {
		return dberror.Wrap(err, "CONSTRAINT_VALIDATION_ERROR", "ValidateInsert", "Validator")
	}

	// Validate each constraint
	for _, constraint := range constraints {
		switch constraint.ConstraintType {
		case systemtable.ConstraintTypeNotNull:
			if err := v.validateNotNull(constraint, tup, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypeCheck:
			if err := v.validateCheck(constraint, tup, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypePrimaryKey:
			// Primary key is a special case of UNIQUE constraint
			if err := v.validateUnique(tx, tableID, constraint, tup, nil, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypeUnique:
			if err := v.validateUnique(tx, tableID, constraint, tup, nil, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypeForeignKey:
			// Foreign key validation requires lookup in referenced table - not implemented yet
			// This is just a placeholder for the validation logic
		}
	}

	return nil
}

// ValidateUpdate validates a tuple update against all enabled constraints.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table being updated
//   - tableName: Name of the table (for error messages)
//   - oldTuple: The tuple before update
//   - newTuple: The tuple after update
//   - sch: The table schema
//
// Returns a DBError if validation fails, nil otherwise.
func (v *Validator) ValidateUpdate(tx operations.TxContext, tableID primitives.FileID, tableName string, oldTuple, newTuple *tuple.Tuple, sch *schema.Schema) error {
	// Get all enabled constraints for the table
	constraints, err := v.constraintOps.GetEnabledConstraintsForTable(tx, tableID)
	if err != nil {
		return dberror.Wrap(err, "CONSTRAINT_VALIDATION_ERROR", "ValidateUpdate", "Validator")
	}

	// Validate each constraint
	for _, constraint := range constraints {
		switch constraint.ConstraintType {
		case systemtable.ConstraintTypeNotNull:
			if err := v.validateNotNull(constraint, newTuple, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypeCheck:
			if err := v.validateCheck(constraint, newTuple, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypePrimaryKey:
			// Primary key is a special case of UNIQUE constraint
			// For updates, we pass the old tuple to allow updating to the same value
			if err := v.validateUnique(tx, tableID, constraint, newTuple, oldTuple, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypeUnique:
			// For updates, we pass the old tuple to allow updating to the same value
			if err := v.validateUnique(tx, tableID, constraint, newTuple, oldTuple, sch, tableName); err != nil {
				return err
			}
		case systemtable.ConstraintTypeForeignKey:
			// Foreign key validation requires lookup in referenced table - not implemented yet
			// This is just a placeholder for the validation logic
		}
	}

	return nil
}

// ValidateDelete validates a tuple deletion against all enabled constraints.
// This primarily checks foreign key constraints in other tables that reference this table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table being deleted from
//   - tableName: Name of the table (for error messages)
//   - tup: The tuple to delete
//   - sch: The table schema
//
// Returns a DBError if validation fails, nil otherwise.
func (v *Validator) ValidateDelete(tx operations.TxContext, tableID primitives.FileID, tableName string, tup *tuple.Tuple, sch *schema.Schema) error {
	// Check if any foreign keys reference this table
	referencingConstraints, err := v.constraintOps.GetForeignKeyConstraintsReferencingTable(tx, tableID)
	if err != nil {
		return dberror.Wrap(err, "CONSTRAINT_VALIDATION_ERROR", "ValidateDelete", "Validator")
	}

	// For each referencing constraint, check if the tuple is referenced
	for _, constraint := range referencingConstraints {
		if !constraint.IsEnabled {
			continue
		}

		// Handle different ON DELETE actions
		switch constraint.OnDeleteAction {
		case "RESTRICT", "NO ACTION":
			// Check if any tuples reference this one - requires lookup in referencing table
			// This is handled by the executor
		case "CASCADE":
			// Delete referencing tuples - handled by executor
		case "SET NULL":
			// Set referencing columns to NULL - handled by executor
		}
	}

	return nil
}

// validateNotNull checks if NOT NULL constraints are satisfied.
func (v *Validator) validateNotNull(constraint *systemtable.ConstraintMetadata, tup *tuple.Tuple, sch *schema.Schema, tableName string) error {
	columnNames := strings.Split(constraint.ColumnNames, ",")

	for _, colName := range columnNames {
		colName = strings.TrimSpace(colName)
		colIdx, err := sch.GetFieldIndex(colName)
		if err != nil {
			return dberror.New(dberror.ErrCategoryUser, "INVALID_CONSTRAINT",
				fmt.Sprintf("Column '%s' in constraint '%s' does not exist in table '%s'",
					colName, constraint.ConstraintName, tableName))
		}

		field, err := tup.GetField(colIdx)
		if err != nil {
			return dberror.Wrap(err, "FIELD_ACCESS_ERROR", "validateNotNull", "Validator")
		}

		if field == nil {
			err := dberror.New(dberror.ErrCategoryUser, "NOT_NULL_VIOLATION",
				fmt.Sprintf("NULL value in column '%s' violates not-null constraint '%s'",
					colName, constraint.ConstraintName))
			err.Detail = fmt.Sprintf("Failing row contains NULL in column '%s'", colName)
			err.Hint = fmt.Sprintf("Column '%s' does not allow NULL values", colName)
			return err
		}
	}

	return nil
}

// validateUnique validates UNIQUE and PRIMARY KEY constraints.
// It checks that no other tuple in the table has the same values for the constraint columns.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraint: The UNIQUE or PRIMARY KEY constraint
//   - newTuple: The tuple being inserted or updated
//   - oldTuple: The old tuple (for UPDATE operations, nil for INSERT)
//   - sch: The table schema
//   - tableName: Name of the table (for error messages)
//
// Returns a DBError if the constraint is violated, nil otherwise.
func (v *Validator) validateUnique(
	tx operations.TxContext,
	tableID primitives.FileID,
	constraint *systemtable.ConstraintMetadata,
	newTuple *tuple.Tuple,
	oldTuple *tuple.Tuple,
	sch *schema.Schema,
	tableName string,
) error {
	// If no index searcher is available, skip validation
	// This allows the validator to work in contexts where indexes aren't available
	if v.indexSearcher == nil {
		return nil
	}

	// Parse column names from the constraint
	columnNames := strings.Split(constraint.ColumnNames, ",")
	if len(columnNames) == 0 {
		return dberror.New(dberror.ErrCategorySystem, "INVALID_CONSTRAINT",
			fmt.Sprintf("UNIQUE constraint '%s' has no columns", constraint.ConstraintName))
	}

	// For now, we only support single-column UNIQUE constraints
	// Multi-column UNIQUE constraints require composite indexes
	if len(columnNames) > 1 {
		// Multi-column unique constraints are not yet supported
		return nil
	}

	columnName := strings.TrimSpace(columnNames[0])

	// Get the column index in the tuple
	colIdx, err := sch.GetFieldIndex(columnName)
	if err != nil {
		return dberror.New(dberror.ErrCategorySystem, "INVALID_CONSTRAINT",
			fmt.Sprintf("Column '%s' in constraint '%s' does not exist in table '%s'",
				columnName, constraint.ConstraintName, tableName))
	}

	// Get the value for the constrained column
	fieldValue, err := newTuple.GetField(colIdx)
	if err != nil {
		return dberror.Wrap(err, "FIELD_ACCESS_ERROR", "validateUnique", "Validator")
	}

	// NULL values are allowed in UNIQUE constraints (but not PRIMARY KEY)
	// Multiple NULL values don't violate uniqueness
	if fieldValue == nil {
		if constraint.ConstraintType == systemtable.ConstraintTypePrimaryKey {
			return NewNotNullViolation(tableName, columnName, constraint.ConstraintName)
		}
		return nil
	}

	// Search the index for existing tuples with this value
	recordIDs, err := v.indexSearcher.SearchIndexForKey(tx, tableID, colIdx, fieldValue)
	if err != nil {
		// If index doesn't exist, we can't validate - skip
		// This allows constraints to exist even if indexes aren't created yet
		return nil
	}

	// Check if any records were found
	if len(recordIDs) > 0 {
		// For UPDATE operations, check if the found record is the same as the old tuple
		if oldTuple != nil && oldTuple.RecordID != nil {
			// If all found records are the old tuple being updated, it's OK
			isAllSameTuple := true
			for _, rid := range recordIDs {
				if !rid.PageID.Equals(oldTuple.RecordID.PageID) || rid.TupleNum != oldTuple.RecordID.TupleNum {
					isAllSameTuple = false
					break
				}
			}
			if isAllSameTuple {
				return nil
			}
		}

		// Duplicate found - return appropriate error
		if constraint.ConstraintType == systemtable.ConstraintTypePrimaryKey {
			return NewPrimaryKeyViolation(tableName, columnNames, constraint.ConstraintName, fieldValue)
		}
		return NewUniqueViolation(tableName, columnNames, constraint.ConstraintName, []types.Field{fieldValue})
	}

	return nil
}

// validateCheck validates CHECK constraints.
// This implements a simple expression evaluator for common CHECK constraint patterns.
//
// Supported patterns:
//   - column_name <op> value (e.g., "age >= 18", "price > 0")
//   - column_name BETWEEN value1 AND value2 (e.g., "quantity BETWEEN 0 AND 100")
//   - column_name IN (value1, value2, ...) (e.g., "status IN ('active', 'pending')")
//
// Parameters:
//   - constraint: The CHECK constraint metadata
//   - tup: The tuple to validate
//   - sch: The table schema
//   - tableName: Name of the table (for error messages)
//
// Returns a DBError if the constraint is violated, nil otherwise.
func (v *Validator) validateCheck(constraint *systemtable.ConstraintMetadata, tup *tuple.Tuple, sch *schema.Schema, tableName string) error {
	if constraint.CheckExpression == "" {
		return nil
	}

	// Evaluate the CHECK expression
	result, err := evaluateCheckExpression(constraint.CheckExpression, tup, sch)
	if err != nil {
		// If we can't evaluate the expression, log a warning and skip validation
		// This allows CHECK constraints with complex expressions to be stored
		// even if evaluation isn't fully implemented
		return nil
	}

	// If the expression evaluates to false, the constraint is violated
	if !result {
		return NewCheckViolation(tableName, constraint.ConstraintName, constraint.CheckExpression)
	}

	return nil
}

// ValidatePrimaryKey validates that a primary key value is unique.
// This requires index lookup and is typically called by the executor.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - tableName: Name of the table (for error messages)
//   - keyValue: The primary key value to check
//   - sch: The table schema
//
// Returns a DBError if the key already exists, nil otherwise.
func (v *Validator) ValidatePrimaryKey(tx operations.TxContext, tableID primitives.FileID, tableName string, keyValue types.Field, sch *schema.Schema) error {
	// This is a placeholder - actual implementation requires index lookup
	// which is handled by the executor
	return nil
}

// ValidateUnique validates that a unique constraint is satisfied.
// This requires index lookup and is typically called by the executor.
//
// Parameters:
//   - tx: Transaction context
//   - constraint: The unique constraint to validate
//   - tableName: Name of the table (for error messages)
//   - values: The column values to check
//   - sch: The table schema
//
// Returns a DBError if the values are not unique, nil otherwise.
func (v *Validator) ValidateUnique(tx operations.TxContext, constraint *systemtable.ConstraintMetadata, tableName string, values []types.Field, sch *schema.Schema) error {
	// This is a placeholder - actual implementation requires index lookup
	// which is handled by the executor
	return nil
}

// ValidateForeignKey validates that a foreign key constraint is satisfied.
// This requires lookup in the referenced table and is typically called by the executor.
//
// Parameters:
//   - tx: Transaction context
//   - constraint: The foreign key constraint to validate
//   - tableName: Name of the table (for error messages)
//   - values: The foreign key column values to check
//   - sch: The table schema
//
// Returns a DBError if the foreign key reference is invalid, nil otherwise.
func (v *Validator) ValidateForeignKey(tx operations.TxContext, constraint *systemtable.ConstraintMetadata, tableName string, values []types.Field, sch *schema.Schema) error {
	// This is a placeholder - actual implementation requires lookup in referenced table
	// which is handled by the executor
	return nil
}

// GetConstraintsForTable is a convenience method to get all constraints for a table.
func (v *Validator) GetConstraintsForTable(tx operations.TxContext, tableID primitives.FileID) ([]*systemtable.ConstraintMetadata, error) {
	return v.constraintOps.GetConstraintsForTable(tx, tableID)
}

// GetEnabledConstraintsForTable is a convenience method to get enabled constraints for a table.
func (v *Validator) GetEnabledConstraintsForTable(tx operations.TxContext, tableID primitives.FileID) ([]*systemtable.ConstraintMetadata, error) {
	return v.constraintOps.GetEnabledConstraintsForTable(tx, tableID)
}

// evaluateCheckExpression evaluates a CHECK constraint expression against a tuple.
// This is a simple expression evaluator that supports common patterns.
//
// Supported patterns:
//   - column_name >= value
//   - column_name > value
//   - column_name <= value
//   - column_name < value
//   - column_name = value
//   - column_name != value or column_name <> value
//   - column_name BETWEEN value1 AND value2
//   - column_name IN (value1, value2, ...)
//
// Returns true if the constraint is satisfied, false otherwise.
// Returns an error if the expression cannot be parsed or evaluated.
func evaluateCheckExpression(expression string, tup *tuple.Tuple, sch *schema.Schema) (bool, error) {
	// Trim and convert to lowercase for case-insensitive matching
	expr := strings.TrimSpace(expression)
	exprLower := strings.ToLower(expr)

	// Try to parse BETWEEN expression: column BETWEEN value1 AND value2
	if strings.Contains(exprLower, " between ") && strings.Contains(exprLower, " and ") {
		return evaluateBetween(expr, tup, sch)
	}

	// Try to parse IN expression: column IN (value1, value2, ...)
	if strings.Contains(exprLower, " in ") && strings.Contains(expr, "(") {
		return evaluateIn(expr, tup, sch)
	}

	// Try to parse comparison expression: column <op> value
	for _, op := range []string{">=", "<=", "<>", "!=", ">", "<", "="} {
		if strings.Contains(expr, op) {
			return evaluateComparison(expr, op, tup, sch)
		}
	}

	// Expression pattern not recognized
	return false, fmt.Errorf("unsupported CHECK expression pattern: %s", expression)
}

// evaluateComparison evaluates a simple comparison expression: column <op> value
func evaluateComparison(expr string, op string, tup *tuple.Tuple, sch *schema.Schema) (bool, error) {
	parts := strings.SplitN(expr, op, 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid comparison expression: %s", expr)
	}

	columnName := strings.TrimSpace(parts[0])
	valueStr := strings.TrimSpace(parts[1])

	// Get column value from tuple
	colIdx, err := sch.GetFieldIndex(columnName)
	if err != nil {
		return false, fmt.Errorf("column '%s' not found", columnName)
	}

	field, err := tup.GetField(colIdx)
	if err != nil {
		return false, err
	}

	// NULL values make the comparison NULL (which we treat as false)
	if field == nil {
		return false, nil
	}

	// Parse the expected value based on field type
	expectedValue, err := parseValue(valueStr, field.Type())
	if err != nil {
		return false, err
	}

	// Compare the values
	return compareFields(field, expectedValue, op)
}

// evaluateBetween evaluates a BETWEEN expression: column BETWEEN value1 AND value2
func evaluateBetween(expr string, tup *tuple.Tuple, sch *schema.Schema) (bool, error) {
	exprLower := strings.ToLower(expr)
	betweenIdx := strings.Index(exprLower, " between ")
	andIdx := strings.Index(exprLower, " and ")

	if betweenIdx == -1 || andIdx == -1 || andIdx <= betweenIdx {
		return false, fmt.Errorf("invalid BETWEEN expression: %s", expr)
	}

	columnName := strings.TrimSpace(expr[:betweenIdx])
	value1Str := strings.TrimSpace(expr[betweenIdx+9 : andIdx])
	value2Str := strings.TrimSpace(expr[andIdx+5:])

	// Get column value from tuple
	colIdx, err := sch.GetFieldIndex(columnName)
	if err != nil {
		return false, fmt.Errorf("column '%s' not found", columnName)
	}

	field, err := tup.GetField(colIdx)
	if err != nil {
		return false, err
	}

	// NULL values make the comparison NULL (which we treat as false)
	if field == nil {
		return false, nil
	}

	// Parse the boundary values
	value1, err := parseValue(value1Str, field.Type())
	if err != nil {
		return false, err
	}

	value2, err := parseValue(value2Str, field.Type())
	if err != nil {
		return false, err
	}

	// Check if field is between value1 and value2 (inclusive)
	ge, err := compareFields(field, value1, ">=")
	if err != nil {
		return false, err
	}

	le, err := compareFields(field, value2, "<=")
	if err != nil {
		return false, err
	}

	return ge && le, nil
}

// evaluateIn evaluates an IN expression: column IN (value1, value2, ...)
func evaluateIn(expr string, tup *tuple.Tuple, sch *schema.Schema) (bool, error) {
	exprLower := strings.ToLower(expr)
	inIdx := strings.Index(exprLower, " in ")
	if inIdx == -1 {
		return false, fmt.Errorf("invalid IN expression: %s", expr)
	}

	columnName := strings.TrimSpace(expr[:inIdx])
	valuesStr := strings.TrimSpace(expr[inIdx+4:])

	// Remove parentheses
	if !strings.HasPrefix(valuesStr, "(") || !strings.HasSuffix(valuesStr, ")") {
		return false, fmt.Errorf("IN values must be in parentheses: %s", expr)
	}
	valuesStr = strings.TrimSpace(valuesStr[1 : len(valuesStr)-1])

	// Get column value from tuple
	colIdx, err := sch.GetFieldIndex(columnName)
	if err != nil {
		return false, fmt.Errorf("column '%s' not found", columnName)
	}

	field, err := tup.GetField(colIdx)
	if err != nil {
		return false, err
	}

	// NULL values are not IN any list
	if field == nil {
		return false, nil
	}

	// Parse each value in the list
	valueStrs := strings.Split(valuesStr, ",")
	for _, valueStr := range valueStrs {
		valueStr = strings.TrimSpace(valueStr)
		value, err := parseValue(valueStr, field.Type())
		if err != nil {
			continue
		}

		// Check if field equals this value
		eq, err := compareFields(field, value, "=")
		if err == nil && eq {
			return true, nil
		}
	}

	return false, nil
}

// parseValue parses a string value into a Field of the specified type
func parseValue(valueStr string, fieldType types.Type) (types.Field, error) {
	// Remove quotes from string values
	if strings.HasPrefix(valueStr, "'") && strings.HasSuffix(valueStr, "'") {
		valueStr = valueStr[1 : len(valueStr)-1]
	}

	switch fieldType {
	case types.IntType:
		var intVal int64
		_, err := fmt.Sscanf(valueStr, "%d", &intVal)
		if err != nil {
			return nil, fmt.Errorf("cannot parse '%s' as integer", valueStr)
		}
		return types.NewIntField(intVal), nil

	case types.StringType:
		// Use a reasonable default max size for string fields
		// This is used for constraint validation, not storage
		const defaultMaxSize = 256
		return types.NewStringField(valueStr, defaultMaxSize), nil

	default:
		return nil, fmt.Errorf("unsupported field type: %v", fieldType)
	}
}

// compareFields compares two fields using the specified operator
func compareFields(field1, field2 types.Field, op string) (bool, error) {
	// Map string operators to Predicate constants
	var predicate primitives.Predicate
	switch op {
	case "=":
		predicate = primitives.Equals
	case "!=", "<>":
		predicate = primitives.NotEqual
	case "<":
		predicate = primitives.LessThan
	case "<=":
		predicate = primitives.LessThanOrEqual
	case ">":
		predicate = primitives.GreaterThan
	case ">=":
		predicate = primitives.GreaterThanOrEqual
	default:
		return false, fmt.Errorf("unsupported operator: %s", op)
	}

	// Use the Compare method with the predicate
	return field1.Compare(predicate, field2)
}
