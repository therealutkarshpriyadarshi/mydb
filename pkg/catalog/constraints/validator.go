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

// Validator handles constraint validation for DML operations.
type Validator struct {
	constraintOps *operations.ConstraintOperations
	columnOps     *operations.ColumnOperations
}

// NewValidator creates a new constraint validator.
//
// Parameters:
//   - constraintOps: Operations for accessing constraint metadata
//   - columnOps: Operations for accessing column metadata
//
// Returns a new Validator instance.
func NewValidator(constraintOps *operations.ConstraintOperations, columnOps *operations.ColumnOperations) *Validator {
	return &Validator{
		constraintOps: constraintOps,
		columnOps:     columnOps,
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
			// Primary key validation requires index lookup - handled by executor
			// This is just a placeholder for the validation logic
		case systemtable.ConstraintTypeUnique:
			// Unique constraint validation requires index lookup - handled by executor
			// This is just a placeholder for the validation logic
		case systemtable.ConstraintTypeForeignKey:
			// Foreign key validation requires lookup in referenced table - handled by executor
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
			// Primary key validation requires index lookup - handled by executor
		case systemtable.ConstraintTypeUnique:
			// Unique constraint validation requires index lookup - handled by executor
		case systemtable.ConstraintTypeForeignKey:
			// Foreign key validation requires lookup in referenced table - handled by executor
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

// validateCheck validates CHECK constraints.
// Note: This is a placeholder. Full CHECK constraint validation requires
// expression evaluation which is not yet implemented.
func (v *Validator) validateCheck(constraint *systemtable.ConstraintMetadata, tup *tuple.Tuple, sch *schema.Schema, tableName string) error {
	// TODO: Implement CHECK constraint validation
	// This requires:
	// 1. Parsing the check expression
	// 2. Evaluating the expression against the tuple
	// 3. Returning an error if the expression evaluates to false

	// For now, we just return nil (no validation)
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
