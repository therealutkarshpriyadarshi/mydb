package systemtable

import (
	"fmt"
	"storemy/pkg/catalog/schema"
	"storemy/pkg/primitives"
	"storemy/pkg/tuple"
	"storemy/pkg/types"
)

// ConstraintType represents the type of constraint.
type ConstraintType int

const (
	// ConstraintTypePrimaryKey represents a PRIMARY KEY constraint.
	// Ensures uniqueness and non-null values for one or more columns.
	ConstraintTypePrimaryKey ConstraintType = iota

	// ConstraintTypeUnique represents a UNIQUE constraint.
	// Ensures uniqueness of values in one or more columns (NULL values allowed).
	ConstraintTypeUnique

	// ConstraintTypeForeignKey represents a FOREIGN KEY constraint.
	// Ensures referential integrity between tables.
	ConstraintTypeForeignKey

	// ConstraintTypeCheck represents a CHECK constraint.
	// Ensures that values satisfy a boolean expression.
	ConstraintTypeCheck

	// ConstraintTypeNotNull represents a NOT NULL constraint.
	// Ensures that a column cannot contain NULL values.
	ConstraintTypeNotNull
)

// String returns a string representation of the constraint type.
func (ct ConstraintType) String() string {
	switch ct {
	case ConstraintTypePrimaryKey:
		return "PRIMARY_KEY"
	case ConstraintTypeUnique:
		return "UNIQUE"
	case ConstraintTypeForeignKey:
		return "FOREIGN_KEY"
	case ConstraintTypeCheck:
		return "CHECK"
	case ConstraintTypeNotNull:
		return "NOT_NULL"
	default:
		return "UNKNOWN"
	}
}

// IsValid returns true if the constraint type is valid.
func (ct ConstraintType) IsValid() bool {
	return ct >= ConstraintTypePrimaryKey && ct <= ConstraintTypeNotNull
}

// ConstraintMetadata holds persisted metadata for a single constraint recorded in the system catalog.
// It is used by the CatalogManager to manage and validate constraints.
type ConstraintMetadata struct {
	ConstraintID   primitives.FileID // Unique identifier for the constraint
	ConstraintName string            // User-defined or auto-generated constraint name
	TableID        primitives.FileID // Table this constraint applies to
	ConstraintType ConstraintType    // Type of constraint (PRIMARY KEY, UNIQUE, etc.)
	ColumnNames    string            // Comma-separated list of column names (for multi-column constraints)

	// Foreign key specific fields
	ReferencedTableID   primitives.FileID // Referenced table (for FOREIGN KEY constraints, 0 otherwise)
	ReferencedColumns   string            // Comma-separated referenced column names (for FOREIGN KEY)
	OnDeleteAction      string            // Action on delete: CASCADE, SET NULL, RESTRICT, NO ACTION
	OnUpdateAction      string            // Action on update: CASCADE, SET NULL, RESTRICT, NO ACTION

	// Check constraint specific field
	CheckExpression string // Boolean expression for CHECK constraints

	// Status flags
	IsEnabled bool // Whether the constraint is currently enforced
}

// ConstraintsTable provides accessors and helpers for the CATALOG_CONSTRAINTS system table.
// Each row represents a constraint on a table with its type, columns, and enforcement rules.
type ConstraintsTable struct {
}

// Schema returns the schema for the CATALOG_CONSTRAINTS system table.
// Schema layout:
//
//	(constraint_id INT PRIMARY KEY, constraint_name STRING, table_id INT,
//	 constraint_type INT, column_names STRING, referenced_table_id INT,
//	 referenced_columns STRING, on_delete_action STRING, on_update_action STRING,
//	 check_expression STRING, is_enabled BOOL)
//
// Notes:
//   - constraint_id is the primary key and must be unique.
//   - constraint_name should be unique within a table.
//   - column_names contains comma-separated column names for multi-column constraints.
//   - referenced_table_id, referenced_columns, on_delete_action, and on_update_action
//     are only used for FOREIGN KEY constraints.
//   - check_expression is only used for CHECK constraints.
func (ct *ConstraintsTable) Schema() *schema.Schema {
	sch, _ := schema.NewSchemaBuilder(InvalidTableID, ct.TableName()).
		AddPrimaryKey("constraint_id", types.Uint64Type).
		AddColumn("constraint_name", types.StringType).
		AddColumn("table_id", types.Uint64Type).
		AddColumn("constraint_type", types.IntType).
		AddColumn("column_names", types.StringType).
		AddColumn("referenced_table_id", types.Uint64Type).
		AddColumn("referenced_columns", types.StringType).
		AddColumn("on_delete_action", types.StringType).
		AddColumn("on_update_action", types.StringType).
		AddColumn("check_expression", types.StringType).
		AddColumn("is_enabled", types.BoolType).
		Build()
	return sch
}

// TableName returns the canonical name of the system table.
func (ct *ConstraintsTable) TableName() string {
	return "CATALOG_CONSTRAINTS"
}

// FileName returns the filename used to persist the CATALOG_CONSTRAINTS heap.
func (ct *ConstraintsTable) FileName() string {
	return "catalog_constraints.dat"
}

// PrimaryKey returns the primary key field name in the schema.
func (ct *ConstraintsTable) PrimaryKey() string {
	return "constraint_id"
}

// GetNumFields returns the number of fields in the CATALOG_CONSTRAINTS schema.
func (ct *ConstraintsTable) GetNumFields() int {
	return 11
}

// CreateTuple constructs a catalog tuple for a given ConstraintMetadata.
// Fields are populated in schema order.
func (ct *ConstraintsTable) CreateTuple(cm ConstraintMetadata) *tuple.Tuple {
	td := ct.Schema().TupleDesc
	return tuple.NewBuilder(td).
		AddUint64(uint64(cm.ConstraintID)).
		AddString(cm.ConstraintName).
		AddUint64(uint64(cm.TableID)).
		AddInt(int64(cm.ConstraintType)).
		AddString(cm.ColumnNames).
		AddUint64(uint64(cm.ReferencedTableID)).
		AddString(cm.ReferencedColumns).
		AddString(cm.OnDeleteAction).
		AddString(cm.OnUpdateAction).
		AddString(cm.CheckExpression).
		AddBool(cm.IsEnabled).
		MustBuild()
}

// GetID extracts the constraint_id from a catalog tuple and validates tuple arity.
// Returns an error when the tuple does not match the expected schema length.
func (ct *ConstraintsTable) GetID(t *tuple.Tuple) (int, error) {
	if int(t.NumFields()) != ct.GetNumFields() {
		return -1, fmt.Errorf("invalid tuple: expected %d fields, got %d", ct.GetNumFields(), t.TupleDesc.NumFields())
	}
	return getIntField(t, 0), nil
}

// TableIDIndex returns the field index where table_id is stored.
func (ct *ConstraintsTable) TableIDIndex() int {
	return 2 // table_id is the 3rd field (0-indexed)
}

// Parse converts a catalog tuple into a ConstraintMetadata instance with validation.
// Validation performed:
//   - Tuple arity matches expected schema length.
//   - constraint_id is not InvalidTableID (reserved).
//   - constraint_name and column_names are non-empty strings.
//   - constraint_type is valid.
//   - Foreign key constraints have valid referenced table and columns.
//   - Check constraints have non-empty expressions.
//
// Returns parsed ConstraintMetadata or an error if validation fails.
func (ct *ConstraintsTable) Parse(t *tuple.Tuple) (*ConstraintMetadata, error) {
	p := tuple.NewParser(t).ExpectFields(ct.GetNumFields())

	constraintID := primitives.FileID(p.ReadUint64())
	constraintName := p.ReadString()
	tableID := primitives.FileID(p.ReadUint64())
	constraintType := ConstraintType(p.ReadInt())
	columnNames := p.ReadString()
	referencedTableID := primitives.FileID(p.ReadUint64())
	referencedColumns := p.ReadString()
	onDeleteAction := p.ReadString()
	onUpdateAction := p.ReadString()
	checkExpression := p.ReadString()
	isEnabled := p.ReadBool()

	if err := p.Error(); err != nil {
		return nil, err
	}

	if constraintID == InvalidTableID {
		return nil, fmt.Errorf("invalid constraint_id: cannot be InvalidTableID (%d)", InvalidTableID)
	}

	if constraintName == "" {
		return nil, fmt.Errorf("constraint_name cannot be empty")
	}

	if tableID == InvalidTableID {
		return nil, fmt.Errorf("invalid table_id: cannot be InvalidTableID (%d)", InvalidTableID)
	}

	if !constraintType.IsValid() {
		return nil, fmt.Errorf("invalid constraint_type: %d", constraintType)
	}

	if columnNames == "" {
		return nil, fmt.Errorf("column_names cannot be empty")
	}

	// Validate foreign key constraints
	if constraintType == ConstraintTypeForeignKey {
		if referencedTableID == InvalidTableID || referencedTableID == 0 {
			return nil, fmt.Errorf("foreign key constraint must have a valid referenced_table_id")
		}
		if referencedColumns == "" {
			return nil, fmt.Errorf("foreign key constraint must have referenced_columns")
		}
	}

	// Validate check constraints
	if constraintType == ConstraintTypeCheck {
		if checkExpression == "" {
			return nil, fmt.Errorf("check constraint must have a check_expression")
		}
	}

	return &ConstraintMetadata{
		ConstraintID:        constraintID,
		ConstraintName:      constraintName,
		TableID:             tableID,
		ConstraintType:      constraintType,
		ColumnNames:         columnNames,
		ReferencedTableID:   referencedTableID,
		ReferencedColumns:   referencedColumns,
		OnDeleteAction:      onDeleteAction,
		OnUpdateAction:      onUpdateAction,
		CheckExpression:     checkExpression,
		IsEnabled:           isEnabled,
	}, nil
}
