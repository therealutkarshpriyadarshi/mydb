package catalogmanager

import (
	"fmt"
	"storemy/pkg/catalog/constraints"
	"storemy/pkg/catalog/systemtable"
	"storemy/pkg/primitives"
)

// ConstraintMetadata is a type alias for easier use
type ConstraintMetadata = systemtable.ConstraintMetadata

// ConstraintType is a type alias for easier use
type ConstraintType = systemtable.ConstraintType

// Constraint type constants (re-exported for convenience)
const (
	ConstraintTypePrimaryKey ConstraintType = systemtable.ConstraintTypePrimaryKey
	ConstraintTypeUnique     ConstraintType = systemtable.ConstraintTypeUnique
	ConstraintTypeForeignKey ConstraintType = systemtable.ConstraintTypeForeignKey
	ConstraintTypeCheck      ConstraintType = systemtable.ConstraintTypeCheck
	ConstraintTypeNotNull    ConstraintType = systemtable.ConstraintTypeNotNull
)

// === Constraint Creation Methods ===

// AddConstraint adds a new constraint to a table.
//
// Parameters:
//   - tx: Transaction context
//   - constraint: The constraint metadata to add
//
// Returns an error if the constraint already exists or insertion fails.
func (cm *CatalogManager) AddConstraint(tx TxContext, constraint *ConstraintMetadata) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate that the table exists
	_, err := cm.tableOps.GetTableMetadataByID(tx, constraint.TableID)
	if err != nil {
		return fmt.Errorf("table %d does not exist: %w", constraint.TableID, err)
	}

	// For foreign key constraints, validate that the referenced table exists
	if constraint.ConstraintType == ConstraintTypeForeignKey {
		_, err := cm.tableOps.GetTableMetadataByID(tx, constraint.ReferencedTableID)
		if err != nil {
			return fmt.Errorf("referenced table %d does not exist: %w", constraint.ReferencedTableID, err)
		}
	}

	return cm.constraintOps.AddConstraint(tx, constraint)
}

// CreatePrimaryKeyConstraint creates a PRIMARY KEY constraint on a table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraintName: Name for the constraint
//   - columnNames: Comma-separated list of column names
//
// Returns the constraint ID or an error.
func (cm *CatalogManager) CreatePrimaryKeyConstraint(tx TxContext, tableID primitives.FileID, constraintName string, columnNames string) (primitives.FileID, error) {
	constraintID := cm.generateConstraintID(tableID, constraintName)
	constraint := &ConstraintMetadata{
		ConstraintID:   constraintID,
		ConstraintName: constraintName,
		TableID:        tableID,
		ConstraintType: ConstraintTypePrimaryKey,
		ColumnNames:    columnNames,
		IsEnabled:      true,
	}

	err := cm.AddConstraint(tx, constraint)
	if err != nil {
		return 0, err
	}

	return constraintID, nil
}

// CreateUniqueConstraint creates a UNIQUE constraint on a table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraintName: Name for the constraint
//   - columnNames: Comma-separated list of column names
//
// Returns the constraint ID or an error.
func (cm *CatalogManager) CreateUniqueConstraint(tx TxContext, tableID primitives.FileID, constraintName string, columnNames string) (primitives.FileID, error) {
	constraintID := cm.generateConstraintID(tableID, constraintName)
	constraint := &ConstraintMetadata{
		ConstraintID:   constraintID,
		ConstraintName: constraintName,
		TableID:        tableID,
		ConstraintType: ConstraintTypeUnique,
		ColumnNames:    columnNames,
		IsEnabled:      true,
	}

	err := cm.AddConstraint(tx, constraint)
	if err != nil {
		return 0, err
	}

	return constraintID, nil
}

// CreateForeignKeyConstraint creates a FOREIGN KEY constraint on a table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table with the foreign key
//   - constraintName: Name for the constraint
//   - columnNames: Comma-separated list of column names
//   - referencedTableID: ID of the referenced table
//   - referencedColumns: Comma-separated list of referenced column names
//   - onDeleteAction: Action on delete (CASCADE, SET NULL, RESTRICT, NO ACTION)
//   - onUpdateAction: Action on update (CASCADE, SET NULL, RESTRICT, NO ACTION)
//
// Returns the constraint ID or an error.
func (cm *CatalogManager) CreateForeignKeyConstraint(tx TxContext, tableID primitives.FileID, constraintName string, columnNames string, referencedTableID primitives.FileID, referencedColumns string, onDeleteAction string, onUpdateAction string) (primitives.FileID, error) {
	constraintID := cm.generateConstraintID(tableID, constraintName)
	constraint := &ConstraintMetadata{
		ConstraintID:        constraintID,
		ConstraintName:      constraintName,
		TableID:             tableID,
		ConstraintType:      ConstraintTypeForeignKey,
		ColumnNames:         columnNames,
		ReferencedTableID:   referencedTableID,
		ReferencedColumns:   referencedColumns,
		OnDeleteAction:      onDeleteAction,
		OnUpdateAction:      onUpdateAction,
		IsEnabled:           true,
	}

	err := cm.AddConstraint(tx, constraint)
	if err != nil {
		return 0, err
	}

	return constraintID, nil
}

// CreateCheckConstraint creates a CHECK constraint on a table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraintName: Name for the constraint
//   - columnNames: Comma-separated list of column names involved in the check
//   - checkExpression: Boolean expression to check
//
// Returns the constraint ID or an error.
func (cm *CatalogManager) CreateCheckConstraint(tx TxContext, tableID primitives.FileID, constraintName string, columnNames string, checkExpression string) (primitives.FileID, error) {
	constraintID := cm.generateConstraintID(tableID, constraintName)
	constraint := &ConstraintMetadata{
		ConstraintID:    constraintID,
		ConstraintName:  constraintName,
		TableID:         tableID,
		ConstraintType:  ConstraintTypeCheck,
		ColumnNames:     columnNames,
		CheckExpression: checkExpression,
		IsEnabled:       true,
	}

	err := cm.AddConstraint(tx, constraint)
	if err != nil {
		return 0, err
	}

	return constraintID, nil
}

// CreateNotNullConstraint creates a NOT NULL constraint on a table column.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraintName: Name for the constraint
//   - columnName: Name of the column
//
// Returns the constraint ID or an error.
func (cm *CatalogManager) CreateNotNullConstraint(tx TxContext, tableID primitives.FileID, constraintName string, columnName string) (primitives.FileID, error) {
	constraintID := cm.generateConstraintID(tableID, constraintName)
	constraint := &ConstraintMetadata{
		ConstraintID:   constraintID,
		ConstraintName: constraintName,
		TableID:        tableID,
		ConstraintType: ConstraintTypeNotNull,
		ColumnNames:    columnName,
		IsEnabled:      true,
	}

	err := cm.AddConstraint(tx, constraint)
	if err != nil {
		return 0, err
	}

	return constraintID, nil
}

// === Constraint Query Methods ===

// GetConstraint retrieves a constraint by ID.
//
// Parameters:
//   - tx: Transaction context
//   - constraintID: ID of the constraint
//
// Returns the constraint metadata or an error if not found.
func (cm *CatalogManager) GetConstraint(tx TxContext, constraintID primitives.FileID) (*ConstraintMetadata, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.constraintOps.GetConstraintByID(tx, constraintID)
}

// GetConstraintByName retrieves a constraint by name within a table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraintName: Name of the constraint
//
// Returns the constraint metadata or an error if not found.
func (cm *CatalogManager) GetConstraintByName(tx TxContext, tableID primitives.FileID, constraintName string) (*ConstraintMetadata, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.constraintOps.GetConstraintByName(tx, tableID, constraintName)
}

// GetConstraintsForTable retrieves all constraints for a specific table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//
// Returns a slice of constraint metadata or an error.
func (cm *CatalogManager) GetConstraintsForTable(tx TxContext, tableID primitives.FileID) ([]*ConstraintMetadata, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.constraintOps.GetConstraintsForTable(tx, tableID)
}

// GetConstraintsByType retrieves all constraints of a specific type for a table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraintType: Type of constraint to filter by
//
// Returns a slice of constraint metadata or an error.
func (cm *CatalogManager) GetConstraintsByType(tx TxContext, tableID primitives.FileID, constraintType ConstraintType) ([]*ConstraintMetadata, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.constraintOps.GetConstraintsByType(tx, tableID, constraintType)
}

// GetForeignKeyConstraintsReferencingTable retrieves all foreign key constraints
// that reference a specific table.
//
// Parameters:
//   - tx: Transaction context
//   - referencedTableID: ID of the referenced table
//
// Returns a slice of foreign key constraint metadata or an error.
func (cm *CatalogManager) GetForeignKeyConstraintsReferencingTable(tx TxContext, referencedTableID primitives.FileID) ([]*ConstraintMetadata, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.constraintOps.GetForeignKeyConstraintsReferencingTable(tx, referencedTableID)
}

// GetEnabledConstraintsForTable retrieves all enabled constraints for a table.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//
// Returns a slice of enabled constraint metadata or an error.
func (cm *CatalogManager) GetEnabledConstraintsForTable(tx TxContext, tableID primitives.FileID) ([]*ConstraintMetadata, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.constraintOps.GetEnabledConstraintsForTable(tx, tableID)
}

// === Constraint Modification Methods ===

// DropConstraint removes a constraint from a table.
//
// Parameters:
//   - tx: Transaction context
//   - constraintID: ID of the constraint to drop
//
// Returns an error if the constraint cannot be dropped.
func (cm *CatalogManager) DropConstraint(tx TxContext, constraintID primitives.FileID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.constraintOps.DeleteConstraint(tx, constraintID)
}

// DropConstraintByName removes a constraint by name.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - constraintName: Name of the constraint to drop
//
// Returns an error if the constraint cannot be dropped.
func (cm *CatalogManager) DropConstraintByName(tx TxContext, tableID primitives.FileID, constraintName string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Find the constraint first
	constraint, err := cm.constraintOps.GetConstraintByName(tx, tableID, constraintName)
	if err != nil {
		return constraints.NewConstraintNotFound(fmt.Sprintf("%d", tableID), constraintName)
	}

	return cm.constraintOps.DeleteConstraint(tx, constraint.ConstraintID)
}

// EnableConstraint enables enforcement of a constraint.
//
// Parameters:
//   - tx: Transaction context
//   - constraintID: ID of the constraint to enable
//
// Returns an error if the constraint cannot be enabled.
func (cm *CatalogManager) EnableConstraint(tx TxContext, constraintID primitives.FileID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.constraintOps.EnableConstraint(tx, constraintID)
}

// DisableConstraint disables enforcement of a constraint.
//
// Parameters:
//   - tx: Transaction context
//   - constraintID: ID of the constraint to disable
//
// Returns an error if the constraint cannot be disabled.
func (cm *CatalogManager) DisableConstraint(tx TxContext, constraintID primitives.FileID) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.constraintOps.DisableConstraint(tx, constraintID)
}

// === Constraint Validation Methods ===

// GetConstraintValidator returns a validator for constraint checking.
// The validator can be used to validate DML operations against constraints.
//
// Returns a new Validator instance.
func (cm *CatalogManager) GetConstraintValidator() *constraints.Validator {
	return constraints.NewValidator(cm.constraintOps, cm.colOps)
}

// generateConstraintID generates a unique ID for a constraint based on table ID and constraint name.
func (cm *CatalogManager) generateConstraintID(tableID primitives.FileID, constraintName string) primitives.FileID {
	// Generate a deterministic ID by hashing table ID + constraint name
	combined := fmt.Sprintf("%d:%s", tableID, constraintName)
	return primitives.Filepath(combined).Hash()
}
