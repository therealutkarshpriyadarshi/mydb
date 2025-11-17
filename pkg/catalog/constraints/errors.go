package constraints

import (
	"fmt"
	dberror "storemy/pkg/error"
	"storemy/pkg/types"
	"strings"
)

// Error codes for constraint violations
const (
	// ErrCodeNotNullViolation indicates a NOT NULL constraint was violated
	ErrCodeNotNullViolation = "NOT_NULL_VIOLATION"

	// ErrCodeUniqueViolation indicates a UNIQUE constraint was violated
	ErrCodeUniqueViolation = "UNIQUE_VIOLATION"

	// ErrCodePrimaryKeyViolation indicates a PRIMARY KEY constraint was violated
	ErrCodePrimaryKeyViolation = "PRIMARY_KEY_VIOLATION"

	// ErrCodeForeignKeyViolation indicates a FOREIGN KEY constraint was violated
	ErrCodeForeignKeyViolation = "FOREIGN_KEY_VIOLATION"

	// ErrCodeCheckViolation indicates a CHECK constraint was violated
	ErrCodeCheckViolation = "CHECK_VIOLATION"

	// ErrCodeConstraintNotFound indicates the constraint does not exist
	ErrCodeConstraintNotFound = "CONSTRAINT_NOT_FOUND"

	// ErrCodeInvalidConstraint indicates the constraint definition is invalid
	ErrCodeInvalidConstraint = "INVALID_CONSTRAINT"

	// ErrCodeConstraintExists indicates the constraint already exists
	ErrCodeConstraintExists = "CONSTRAINT_EXISTS"
)

// NewNotNullViolation creates a DBError for NOT NULL constraint violations.
//
// Parameters:
//   - tableName: Name of the table
//   - columnName: Name of the column with NOT NULL constraint
//   - constraintName: Name of the constraint
//
// Returns a DBError with appropriate context.
func NewNotNullViolation(tableName, columnName, constraintName string) *dberror.DBError {
	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeNotNullViolation,
		fmt.Sprintf("NULL value in column '%s' violates not-null constraint", columnName),
	)
	err.Detail = fmt.Sprintf("Column '%s.%s' does not allow NULL values", tableName, columnName)
	err.Hint = fmt.Sprintf("Provide a non-NULL value for column '%s'", columnName)
	err.Operation = "INSERT/UPDATE"
	err.Component = "ConstraintValidator"
	return err
}

// NewPrimaryKeyViolation creates a DBError for PRIMARY KEY constraint violations.
//
// Parameters:
//   - tableName: Name of the table
//   - columnNames: Names of the primary key columns
//   - constraintName: Name of the constraint
//   - keyValue: The duplicate key value
//
// Returns a DBError with appropriate context.
func NewPrimaryKeyViolation(tableName string, columnNames []string, constraintName string, keyValue types.Field) *dberror.DBError {
	columns := strings.Join(columnNames, ", ")
	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodePrimaryKeyViolation,
		fmt.Sprintf("Duplicate key value violates primary key constraint '%s'", constraintName),
	)
	err.Detail = fmt.Sprintf("Key (%s)=(%v) already exists in table '%s'", columns, keyValue, tableName)
	err.Hint = "Ensure the primary key value is unique"
	err.Operation = "INSERT/UPDATE"
	err.Component = "ConstraintValidator"
	return err
}

// NewUniqueViolation creates a DBError for UNIQUE constraint violations.
//
// Parameters:
//   - tableName: Name of the table
//   - columnNames: Names of the columns with UNIQUE constraint
//   - constraintName: Name of the constraint
//   - values: The duplicate values
//
// Returns a DBError with appropriate context.
func NewUniqueViolation(tableName string, columnNames []string, constraintName string, values []types.Field) *dberror.DBError {
	columns := strings.Join(columnNames, ", ")
	valueStrs := make([]string, len(values))
	for i, v := range values {
		valueStrs[i] = fmt.Sprintf("%v", v)
	}
	valuesStr := strings.Join(valueStrs, ", ")

	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeUniqueViolation,
		fmt.Sprintf("Duplicate key value violates unique constraint '%s'", constraintName),
	)
	err.Detail = fmt.Sprintf("Key (%s)=(%s) already exists in table '%s'", columns, valuesStr, tableName)
	err.Hint = "Ensure the column values are unique"
	err.Operation = "INSERT/UPDATE"
	err.Component = "ConstraintValidator"
	return err
}

// NewForeignKeyViolation creates a DBError for FOREIGN KEY constraint violations.
//
// Parameters:
//   - tableName: Name of the table with the foreign key
//   - columnNames: Names of the foreign key columns
//   - referencedTable: Name of the referenced table
//   - referencedColumns: Names of the referenced columns
//   - constraintName: Name of the constraint
//   - values: The foreign key values that don't exist in referenced table
//
// Returns a DBError with appropriate context.
func NewForeignKeyViolation(tableName string, columnNames []string, referencedTable string, referencedColumns []string, constraintName string, values []types.Field) *dberror.DBError {
	columns := strings.Join(columnNames, ", ")
	refColumns := strings.Join(referencedColumns, ", ")
	valueStrs := make([]string, len(values))
	for i, v := range values {
		valueStrs[i] = fmt.Sprintf("%v", v)
	}
	valuesStr := strings.Join(valueStrs, ", ")

	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeForeignKeyViolation,
		fmt.Sprintf("Insert or update violates foreign key constraint '%s'", constraintName),
	)
	err.Detail = fmt.Sprintf("Key (%s)=(%s) is not present in table '%s' (%s)", columns, valuesStr, referencedTable, refColumns)
	err.Hint = fmt.Sprintf("Ensure the referenced key exists in table '%s'", referencedTable)
	err.Operation = "INSERT/UPDATE"
	err.Component = "ConstraintValidator"
	return err
}

// NewForeignKeyRestrictionViolation creates a DBError for foreign key RESTRICT/NO ACTION violations on delete.
//
// Parameters:
//   - tableName: Name of the table being deleted from
//   - referencingTable: Name of the table with the foreign key
//   - constraintName: Name of the constraint
//
// Returns a DBError with appropriate context.
func NewForeignKeyRestrictionViolation(tableName, referencingTable, constraintName string) *dberror.DBError {
	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeForeignKeyViolation,
		fmt.Sprintf("Delete violates foreign key constraint '%s' on table '%s'", constraintName, referencingTable),
	)
	err.Detail = fmt.Sprintf("Key is still referenced from table '%s'", referencingTable)
	err.Hint = fmt.Sprintf("Delete or update the referencing rows in table '%s' first, or use CASCADE", referencingTable)
	err.Operation = "DELETE"
	err.Component = "ConstraintValidator"
	return err
}

// NewCheckViolation creates a DBError for CHECK constraint violations.
//
// Parameters:
//   - tableName: Name of the table
//   - constraintName: Name of the constraint
//   - checkExpression: The check expression that failed
//
// Returns a DBError with appropriate context.
func NewCheckViolation(tableName, constraintName, checkExpression string) *dberror.DBError {
	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeCheckViolation,
		fmt.Sprintf("New row violates check constraint '%s'", constraintName),
	)
	err.Detail = fmt.Sprintf("Failing row violates check constraint '%s' on table '%s'", constraintName, tableName)
	err.Hint = fmt.Sprintf("Ensure the row satisfies the check expression: %s", checkExpression)
	err.Operation = "INSERT/UPDATE"
	err.Component = "ConstraintValidator"
	return err
}

// NewConstraintNotFound creates a DBError when a constraint cannot be found.
//
// Parameters:
//   - tableName: Name of the table
//   - constraintName: Name of the constraint
//
// Returns a DBError with appropriate context.
func NewConstraintNotFound(tableName, constraintName string) *dberror.DBError {
	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeConstraintNotFound,
		fmt.Sprintf("Constraint '%s' does not exist", constraintName),
	)
	err.Detail = fmt.Sprintf("Constraint '%s' on table '%s' not found", constraintName, tableName)
	err.Hint = "Check the constraint name and table"
	err.Operation = "ALTER TABLE"
	err.Component = "CatalogManager"
	return err
}

// NewConstraintExists creates a DBError when trying to create a duplicate constraint.
//
// Parameters:
//   - tableName: Name of the table
//   - constraintName: Name of the constraint
//
// Returns a DBError with appropriate context.
func NewConstraintExists(tableName, constraintName string) *dberror.DBError {
	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeConstraintExists,
		fmt.Sprintf("Constraint '%s' already exists", constraintName),
	)
	err.Detail = fmt.Sprintf("Constraint '%s' on table '%s' already exists", constraintName, tableName)
	err.Hint = "Use a different constraint name or drop the existing constraint first"
	err.Operation = "ALTER TABLE"
	err.Component = "CatalogManager"
	return err
}

// NewInvalidConstraint creates a DBError for invalid constraint definitions.
//
// Parameters:
//   - tableName: Name of the table
//   - constraintName: Name of the constraint
//   - reason: Reason why the constraint is invalid
//
// Returns a DBError with appropriate context.
func NewInvalidConstraint(tableName, constraintName, reason string) *dberror.DBError {
	err := dberror.New(
		dberror.ErrCategoryUser,
		ErrCodeInvalidConstraint,
		fmt.Sprintf("Invalid constraint definition for '%s'", constraintName),
	)
	err.Detail = fmt.Sprintf("Constraint '%s' on table '%s' is invalid: %s", constraintName, tableName, reason)
	err.Hint = "Check the constraint definition syntax"
	err.Operation = "ALTER TABLE"
	err.Component = "CatalogManager"
	return err
}
