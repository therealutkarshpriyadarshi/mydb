package operations

import (
	"fmt"
	"storemy/pkg/catalog/catalogio"
	"storemy/pkg/catalog/systemtable"
	"storemy/pkg/primitives"
	"storemy/pkg/tuple"
	"strings"
)

// ConstraintOperations provides operations for managing constraint metadata in CATALOG_CONSTRAINTS.
type ConstraintOperations struct {
	*BaseOperations[*systemtable.ConstraintMetadata]
}

// NewConstraintOperations creates a new ConstraintOperations instance.
//
// Parameters:
//   - access: CatalogAccess for reading and writing catalog data
//   - tableID: ID of the CATALOG_CONSTRAINTS system table
//
// Returns a new ConstraintOperations instance configured for constraint management.
func NewConstraintOperations(access catalogio.CatalogAccess, tableID primitives.FileID) *ConstraintOperations {
	baseOp := NewBaseOperations(access, tableID, systemtable.Constraints.Parse, func(cm *systemtable.ConstraintMetadata) *tuple.Tuple {
		return systemtable.Constraints.CreateTuple(*cm)
	})
	return &ConstraintOperations{
		BaseOperations: baseOp,
	}
}

// GetConstraintByID retrieves a constraint by its unique constraint ID.
//
// Parameters:
//   - tx: Transaction context for reading catalog
//   - constraintID: Unique identifier of the constraint
//
// Returns the ConstraintMetadata or an error if not found.
func (co *ConstraintOperations) GetConstraintByID(tx TxContext, constraintID primitives.FileID) (*systemtable.ConstraintMetadata, error) {
	return co.FindOne(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.ConstraintID == constraintID
	})
}

// GetConstraintByName retrieves a constraint by its name within a specific table.
// Constraint names are case-insensitive.
//
// Parameters:
//   - tx: Transaction context for reading catalog
//   - tableID: ID of the table the constraint belongs to
//   - constraintName: Name of the constraint
//
// Returns the ConstraintMetadata or an error if not found.
func (co *ConstraintOperations) GetConstraintByName(tx TxContext, tableID primitives.FileID, constraintName string) (*systemtable.ConstraintMetadata, error) {
	return co.FindOne(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.TableID == tableID && strings.EqualFold(cm.ConstraintName, constraintName)
	})
}

// GetConstraintsForTable retrieves all constraints defined on a specific table.
//
// Parameters:
//   - tx: Transaction context for reading catalog
//   - tableID: ID of the table
//
// Returns a slice of ConstraintMetadata for all constraints on the table.
func (co *ConstraintOperations) GetConstraintsForTable(tx TxContext, tableID primitives.FileID) ([]*systemtable.ConstraintMetadata, error) {
	return co.FindAll(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.TableID == tableID
	})
}

// GetConstraintsByType retrieves all constraints of a specific type for a table.
//
// Parameters:
//   - tx: Transaction context for reading catalog
//   - tableID: ID of the table
//   - constraintType: Type of constraint to filter by
//
// Returns a slice of ConstraintMetadata matching the specified type.
func (co *ConstraintOperations) GetConstraintsByType(tx TxContext, tableID primitives.FileID, constraintType systemtable.ConstraintType) ([]*systemtable.ConstraintMetadata, error) {
	return co.FindAll(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.TableID == tableID && cm.ConstraintType == constraintType
	})
}

// GetForeignKeyConstraintsReferencingTable retrieves all foreign key constraints
// that reference a specific table. This is useful for enforcing referential integrity
// when deleting or updating rows in the referenced table.
//
// Parameters:
//   - tx: Transaction context for reading catalog
//   - referencedTableID: ID of the table being referenced
//
// Returns a slice of ConstraintMetadata for all foreign keys referencing the table.
func (co *ConstraintOperations) GetForeignKeyConstraintsReferencingTable(tx TxContext, referencedTableID primitives.FileID) ([]*systemtable.ConstraintMetadata, error) {
	return co.FindAll(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.ConstraintType == systemtable.ConstraintTypeForeignKey && cm.ReferencedTableID == referencedTableID
	})
}

// GetEnabledConstraintsForTable retrieves all enabled constraints for a table.
// Only enabled constraints are enforced during DML operations.
//
// Parameters:
//   - tx: Transaction context for reading catalog
//   - tableID: ID of the table
//
// Returns a slice of enabled ConstraintMetadata for the table.
func (co *ConstraintOperations) GetEnabledConstraintsForTable(tx TxContext, tableID primitives.FileID) ([]*systemtable.ConstraintMetadata, error) {
	return co.FindAll(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.TableID == tableID && cm.IsEnabled
	})
}

// AddConstraint adds a new constraint to the catalog.
//
// Parameters:
//   - tx: Transaction context for catalog modification
//   - constraint: The constraint metadata to add
//
// Returns an error if the constraint already exists or insertion fails.
func (co *ConstraintOperations) AddConstraint(tx TxContext, constraint *systemtable.ConstraintMetadata) error {
	// Check if constraint with same name already exists on the table
	existing, err := co.GetConstraintByName(tx, constraint.TableID, constraint.ConstraintName)
	if err == nil && existing != nil {
		return fmt.Errorf("constraint '%s' already exists on table %d", constraint.ConstraintName, constraint.TableID)
	}

	return co.Insert(tx, constraint)
}

// DeleteConstraint removes a constraint from the catalog by its ID.
//
// Parameters:
//   - tx: Transaction context for catalog modification
//   - constraintID: ID of the constraint to remove
//
// Returns an error if deletion fails or constraint not found.
func (co *ConstraintOperations) DeleteConstraint(tx TxContext, constraintID primitives.FileID) error {
	return co.DeleteBy(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.ConstraintID == constraintID
	})
}

// DeleteConstraintsForTable removes all constraints associated with a table.
// This is typically called when dropping a table.
//
// Parameters:
//   - tx: Transaction context for catalog modification
//   - tableID: ID of the table whose constraints should be deleted
//
// Returns an error if deletion fails.
func (co *ConstraintOperations) DeleteConstraintsForTable(tx TxContext, tableID primitives.FileID) error {
	return co.DeleteBy(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.TableID == tableID
	})
}

// EnableConstraint enables enforcement of a constraint.
//
// Parameters:
//   - tx: Transaction context for catalog modification
//   - constraintID: ID of the constraint to enable
//
// Returns an error if the constraint is not found or update fails.
func (co *ConstraintOperations) EnableConstraint(tx TxContext, constraintID primitives.FileID) error {
	return co.UpdateBy(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.ConstraintID == constraintID
	}, func(cm *systemtable.ConstraintMetadata) *systemtable.ConstraintMetadata {
		cm.IsEnabled = true
		return cm
	})
}

// DisableConstraint disables enforcement of a constraint.
//
// Parameters:
//   - tx: Transaction context for catalog modification
//   - constraintID: ID of the constraint to disable
//
// Returns an error if the constraint is not found or update fails.
func (co *ConstraintOperations) DisableConstraint(tx TxContext, constraintID primitives.FileID) error {
	return co.UpdateBy(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.ConstraintID == constraintID
	}, func(cm *systemtable.ConstraintMetadata) *systemtable.ConstraintMetadata {
		cm.IsEnabled = false
		return cm
	})
}

// UpdateConstraint updates an existing constraint with new metadata.
// Uses the delete-then-insert pattern for MVCC compatibility.
//
// Parameters:
//   - tx: Transaction context for catalog modification
//   - constraintID: ID of the constraint to update
//   - updatedConstraint: New constraint metadata
//
// Returns an error if the constraint is not found or update fails.
func (co *ConstraintOperations) UpdateConstraint(tx TxContext, constraintID primitives.FileID, updatedConstraint *systemtable.ConstraintMetadata) error {
	return co.Upsert(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return cm.ConstraintID == constraintID
	}, updatedConstraint)
}

// GetAllConstraints retrieves metadata for all constraints in the database.
//
// Parameters:
//   - tx: Transaction context for reading catalog
//
// Returns a slice of all ConstraintMetadata or an error if catalog read fails.
func (co *ConstraintOperations) GetAllConstraints(tx TxContext) ([]*systemtable.ConstraintMetadata, error) {
	return co.FindAll(tx, func(cm *systemtable.ConstraintMetadata) bool {
		return true
	})
}
