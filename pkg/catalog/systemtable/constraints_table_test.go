package systemtable

import (
	"storemy/pkg/primitives"
	"testing"
)

func TestConstraintsTable_Schema(t *testing.T) {
	ct := &ConstraintsTable{}
	schema := ct.Schema()

	if schema == nil {
		t.Fatal("Schema should not be nil")
	}

	expectedFields := primitives.ColumnID(11)
	if schema.TupleDesc.NumFields() != expectedFields {
		t.Errorf("Expected %d fields, got %d", expectedFields, schema.TupleDesc.NumFields())
	}

	if ct.TableName() != "CATALOG_CONSTRAINTS" {
		t.Errorf("Expected table name 'CATALOG_CONSTRAINTS', got '%s'", ct.TableName())
	}

	if ct.FileName() != "catalog_constraints.dat" {
		t.Errorf("Expected filename 'catalog_constraints.dat', got '%s'", ct.FileName())
	}

	if ct.PrimaryKey() != "constraint_id" {
		t.Errorf("Expected primary key 'constraint_id', got '%s'", ct.PrimaryKey())
	}
}

func TestConstraintsTable_CreateAndParse_PrimaryKey(t *testing.T) {
	ct := &ConstraintsTable{}

	constraint := ConstraintMetadata{
		ConstraintID:   primitives.FileID(1),
		ConstraintName: "pk_users",
		TableID:        primitives.FileID(10),
		ConstraintType: ConstraintTypePrimaryKey,
		ColumnNames:    "id",
		IsEnabled:      true,
	}

	tuple := ct.CreateTuple(constraint)
	if tuple == nil {
		t.Fatal("CreateTuple returned nil")
	}

	parsed, err := ct.Parse(tuple)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.ConstraintID != constraint.ConstraintID {
		t.Errorf("Expected constraint ID %d, got %d", constraint.ConstraintID, parsed.ConstraintID)
	}

	if parsed.ConstraintName != constraint.ConstraintName {
		t.Errorf("Expected constraint name '%s', got '%s'", constraint.ConstraintName, parsed.ConstraintName)
	}

	if parsed.TableID != constraint.TableID {
		t.Errorf("Expected table ID %d, got %d", constraint.TableID, parsed.TableID)
	}

	if parsed.ConstraintType != constraint.ConstraintType {
		t.Errorf("Expected constraint type %v, got %v", constraint.ConstraintType, parsed.ConstraintType)
	}

	if parsed.ColumnNames != constraint.ColumnNames {
		t.Errorf("Expected column names '%s', got '%s'", constraint.ColumnNames, parsed.ColumnNames)
	}

	if parsed.IsEnabled != constraint.IsEnabled {
		t.Errorf("Expected is_enabled %v, got %v", constraint.IsEnabled, parsed.IsEnabled)
	}
}

func TestConstraintsTable_CreateAndParse_ForeignKey(t *testing.T) {
	ct := &ConstraintsTable{}

	constraint := ConstraintMetadata{
		ConstraintID:      primitives.FileID(2),
		ConstraintName:    "fk_orders_users",
		TableID:           primitives.FileID(20),
		ConstraintType:    ConstraintTypeForeignKey,
		ColumnNames:       "user_id",
		ReferencedTableID: primitives.FileID(10),
		ReferencedColumns: "id",
		OnDeleteAction:    "CASCADE",
		OnUpdateAction:    "NO ACTION",
		IsEnabled:         true,
	}

	tuple := ct.CreateTuple(constraint)
	parsed, err := ct.Parse(tuple)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.ReferencedTableID != constraint.ReferencedTableID {
		t.Errorf("Expected referenced table ID %d, got %d", constraint.ReferencedTableID, parsed.ReferencedTableID)
	}

	if parsed.ReferencedColumns != constraint.ReferencedColumns {
		t.Errorf("Expected referenced columns '%s', got '%s'", constraint.ReferencedColumns, parsed.ReferencedColumns)
	}

	if parsed.OnDeleteAction != constraint.OnDeleteAction {
		t.Errorf("Expected on delete action '%s', got '%s'", constraint.OnDeleteAction, parsed.OnDeleteAction)
	}

	if parsed.OnUpdateAction != constraint.OnUpdateAction {
		t.Errorf("Expected on update action '%s', got '%s'", constraint.OnUpdateAction, parsed.OnUpdateAction)
	}
}

func TestConstraintsTable_CreateAndParse_Check(t *testing.T) {
	ct := &ConstraintsTable{}

	constraint := ConstraintMetadata{
		ConstraintID:    primitives.FileID(3),
		ConstraintName:  "check_age",
		TableID:         primitives.FileID(10),
		ConstraintType:  ConstraintTypeCheck,
		ColumnNames:     "age",
		CheckExpression: "age >= 18",
		IsEnabled:       true,
	}

	tuple := ct.CreateTuple(constraint)
	parsed, err := ct.Parse(tuple)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.CheckExpression != constraint.CheckExpression {
		t.Errorf("Expected check expression '%s', got '%s'", constraint.CheckExpression, parsed.CheckExpression)
	}
}

func TestConstraintsTable_Parse_ValidationErrors(t *testing.T) {
	ct := &ConstraintsTable{}

	tests := []struct {
		name        string
		constraint  ConstraintMetadata
		expectError bool
	}{
		{
			name: "Invalid constraint ID",
			constraint: ConstraintMetadata{
				ConstraintID:   InvalidTableID,
				ConstraintName: "test",
				TableID:        primitives.FileID(10),
				ConstraintType: ConstraintTypePrimaryKey,
				ColumnNames:    "id",
				IsEnabled:      true,
			},
			expectError: true,
		},
		{
			name: "Empty constraint name",
			constraint: ConstraintMetadata{
				ConstraintID:   primitives.FileID(1),
				ConstraintName: "",
				TableID:        primitives.FileID(10),
				ConstraintType: ConstraintTypePrimaryKey,
				ColumnNames:    "id",
				IsEnabled:      true,
			},
			expectError: true,
		},
		{
			name: "Invalid table ID",
			constraint: ConstraintMetadata{
				ConstraintID:   primitives.FileID(1),
				ConstraintName: "test",
				TableID:        InvalidTableID,
				ConstraintType: ConstraintTypePrimaryKey,
				ColumnNames:    "id",
				IsEnabled:      true,
			},
			expectError: true,
		},
		{
			name: "Empty column names",
			constraint: ConstraintMetadata{
				ConstraintID:   primitives.FileID(1),
				ConstraintName: "test",
				TableID:        primitives.FileID(10),
				ConstraintType: ConstraintTypePrimaryKey,
				ColumnNames:    "",
				IsEnabled:      true,
			},
			expectError: true,
		},
		{
			name: "Foreign key without referenced table",
			constraint: ConstraintMetadata{
				ConstraintID:      primitives.FileID(1),
				ConstraintName:    "test_fk",
				TableID:           primitives.FileID(10),
				ConstraintType:    ConstraintTypeForeignKey,
				ColumnNames:       "user_id",
				ReferencedTableID: InvalidTableID,
				ReferencedColumns: "id",
				IsEnabled:         true,
			},
			expectError: true,
		},
		{
			name: "Foreign key without referenced columns",
			constraint: ConstraintMetadata{
				ConstraintID:      primitives.FileID(1),
				ConstraintName:    "test_fk",
				TableID:           primitives.FileID(10),
				ConstraintType:    ConstraintTypeForeignKey,
				ColumnNames:       "user_id",
				ReferencedTableID: primitives.FileID(20),
				ReferencedColumns: "",
				IsEnabled:         true,
			},
			expectError: true,
		},
		{
			name: "Check constraint without expression",
			constraint: ConstraintMetadata{
				ConstraintID:    primitives.FileID(1),
				ConstraintName:  "test_check",
				TableID:         primitives.FileID(10),
				ConstraintType:  ConstraintTypeCheck,
				ColumnNames:     "age",
				CheckExpression: "",
				IsEnabled:       true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tuple := ct.CreateTuple(tt.constraint)
			_, err := ct.Parse(tuple)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestConstraintType_String(t *testing.T) {
	tests := []struct {
		constraintType ConstraintType
		expected       string
	}{
		{ConstraintTypePrimaryKey, "PRIMARY_KEY"},
		{ConstraintTypeUnique, "UNIQUE"},
		{ConstraintTypeForeignKey, "FOREIGN_KEY"},
		{ConstraintTypeCheck, "CHECK"},
		{ConstraintTypeNotNull, "NOT_NULL"},
		{ConstraintType(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.constraintType.String()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestConstraintType_IsValid(t *testing.T) {
	tests := []struct {
		constraintType ConstraintType
		expected       bool
	}{
		{ConstraintTypePrimaryKey, true},
		{ConstraintTypeUnique, true},
		{ConstraintTypeForeignKey, true},
		{ConstraintTypeCheck, true},
		{ConstraintTypeNotNull, true},
		{ConstraintType(-1), false},
		{ConstraintType(999), false},
	}

	for _, tt := range tests {
		t.Run(tt.constraintType.String(), func(t *testing.T) {
			result := tt.constraintType.IsValid()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
