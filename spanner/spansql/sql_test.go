/*
Copyright 2019 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spansql

import (
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/civil"
)

func boolAddr(b bool) *bool {
	return &b
}

func TestSQL(t *testing.T) {
	reparseDDL := func(s string) (interface{}, error) {
		ddl, err := ParseDDLStmt(s)
		if err != nil {
			return nil, err
		}
		ddl.clearOffset()
		return ddl, nil
	}
	reparseDML := func(s string) (interface{}, error) {
		dml, err := ParseDMLStmt(s)
		if err != nil {
			return nil, err
		}
		return dml, nil
	}
	reparseQuery := func(s string) (interface{}, error) {
		q, err := ParseQuery(s)
		return q, err
	}
	reparseExpr := func(s string) (interface{}, error) {
		e, pe := newParser("f-expr", s).parseExpr()
		if pe != nil {
			return nil, pe
		}
		return e, nil
	}

	latz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("Loading Los Angeles time zone info: %v", err)
	}

	line := func(n int) Position { return Position{Line: n} }
	tests := []struct {
		data    interface{ SQL() string }
		sql     string
		reparse func(string) (interface{}, error)
	}{
		{
			&CreateTable{
				Name: "Ta",
				Columns: []ColumnDef{
					{Name: "Ca", Type: Type{Base: Bool}, NotNull: true, Position: line(2)},
					{Name: "Cb", Type: Type{Base: Int64}, Position: line(3)},
					{Name: "Cc", Type: Type{Base: Float64}, Position: line(4)},
					{Name: "Cd", Type: Type{Base: String, Len: 17}, Position: line(5)},
					{Name: "Ce", Type: Type{Base: String, Len: MaxLen}, Position: line(6)},
					{Name: "Cf", Type: Type{Base: Bytes, Len: 4711}, Position: line(7)},
					{Name: "Cg", Type: Type{Base: Bytes, Len: MaxLen}, Position: line(8)},
					{Name: "Ch", Type: Type{Base: Date}, Position: line(9)},
					{Name: "Ci", Type: Type{Base: Timestamp}, Options: ColumnOptions{AllowCommitTimestamp: boolAddr(true)}, Position: line(10)},
					{Name: "Cj", Type: Type{Array: true, Base: Int64}, Position: line(11)},
					{Name: "Ck", Type: Type{Array: true, Base: String, Len: MaxLen}, Position: line(12)},
					{Name: "Cl", Type: Type{Base: Timestamp}, Options: ColumnOptions{AllowCommitTimestamp: boolAddr(false)}, Position: line(13)},
					{Name: "Cm", Type: Type{Base: Int64}, Generated: Func{Name: "CHAR_LENGTH", Args: []Expr{ID("Ce")}}, Position: line(14)},
					{Name: "Cn", Type: Type{Base: JSON}, Position: line(15)},
					{Name: "Co", Type: Type{Base: Int64}, Default: IntegerLiteral(1), Position: line(16)},
				},
				PrimaryKey: []KeyPart{
					{Column: "Ca"},
					{Column: "Cb", Desc: true},
				},
				Position: line(1),
			},
			`CREATE TABLE Ta (
  Ca BOOL NOT NULL,
  Cb INT64,
  Cc FLOAT64,
  Cd STRING(17),
  Ce STRING(MAX),
  Cf BYTES(4711),
  Cg BYTES(MAX),
  Ch DATE,
  Ci TIMESTAMP OPTIONS (allow_commit_timestamp = true),
  Cj ARRAY<INT64>,
  Ck ARRAY<STRING(MAX)>,
  Cl TIMESTAMP OPTIONS (allow_commit_timestamp = null),
  Cm INT64 AS (CHAR_LENGTH(Ce)) STORED,
  Cn JSON,
  Co INT64 DEFAULT (1),
) PRIMARY KEY(Ca, Cb DESC)`,
			reparseDDL,
		},
		{
			&CreateTable{
				Name: "Tsub",
				Columns: []ColumnDef{
					{Name: "SomeId", Type: Type{Base: Int64}, NotNull: true, Position: line(2)},
					{Name: "OtherId", Type: Type{Base: Int64}, NotNull: true, Position: line(3)},
					// This column name uses a reserved keyword.
					{Name: "Hash", Type: Type{Base: Bytes, Len: 32}, Position: line(4)},
				},
				PrimaryKey: []KeyPart{
					{Column: "SomeId"},
					{Column: "OtherId"},
				},
				Interleave: &Interleave{
					Parent:   "Ta",
					OnDelete: CascadeOnDelete,
				},
				Position: line(1),
			},
			`CREATE TABLE Tsub (
  SomeId INT64 NOT NULL,
  OtherId INT64 NOT NULL,
  ` + "`Hash`" + ` BYTES(32),
) PRIMARY KEY(SomeId, OtherId),
  INTERLEAVE IN PARENT Ta ON DELETE CASCADE`,
			reparseDDL,
		},
		{
			&CreateTable{
				Name: "WithRowDeletionPolicy",
				Columns: []ColumnDef{
					{Name: "Name", Type: Type{Base: String, Len: MaxLen}, NotNull: true, Position: line(2)},
					{Name: "DelTimestamp", Type: Type{Base: Timestamp}, NotNull: true, Position: line(3)},
				},
				PrimaryKey: []KeyPart{{Column: "Name"}},
				RowDeletionPolicy: &RowDeletionPolicy{
					Column:  ID("DelTimestamp"),
					NumDays: 30,
				},
				Position: line(1),
			},
			`CREATE TABLE WithRowDeletionPolicy (
  Name STRING(MAX) NOT NULL,
  DelTimestamp TIMESTAMP NOT NULL,
) PRIMARY KEY(Name),
  ROW DELETION POLICY ( OLDER_THAN ( DelTimestamp, INTERVAL 30 DAY ))`,
			reparseDDL,
		},
		{
			&DropTable{
				Name:     "Ta",
				Position: line(1),
			},
			"DROP TABLE Ta",
			reparseDDL,
		},
		{
			&CreateIndex{
				Name:  "Ia",
				Table: "Ta",
				Columns: []KeyPart{
					{Column: "Ca"},
					{Column: "Cb", Desc: true},
				},
				Position: line(1),
			},
			"CREATE INDEX Ia ON Ta(Ca, Cb DESC)",
			reparseDDL,
		},
		{
			&DropIndex{
				Name:     "Ia",
				Position: line(1),
			},
			"DROP INDEX Ia",
			reparseDDL,
		},
		{
			&CreateView{
				Name:      "SingersView",
				OrReplace: true,
				Query: Query{
					Select: Select{
						List: []Expr{ID("SingerId"), ID("FullName"), ID("Picture")},
						From: []SelectFrom{SelectFromTable{
							Table: "Singers",
						}},
					},
					Order: []Order{
						{Expr: ID("LastName")},
						{Expr: ID("FirstName")},
					},
				},
				Position: line(1),
			},
			"CREATE OR REPLACE VIEW SingersView SQL SECURITY INVOKER AS SELECT SingerId, FullName, Picture FROM Singers ORDER BY LastName, FirstName",
			reparseDDL,
		},
		{
			&DropView{
				Name:     "SingersView",
				Position: line(1),
			},
			"DROP VIEW SingersView",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: AddColumn{Def: ColumnDef{Name: "Ca", Type: Type{Base: Bool}, Position: line(1)}},
				Position:   line(1),
			},
			"ALTER TABLE Ta ADD COLUMN Ca BOOL",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: DropColumn{Name: "Ca"},
				Position:   line(1),
			},
			"ALTER TABLE Ta DROP COLUMN Ca",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: SetOnDelete{Action: NoActionOnDelete},
				Position:   line(1),
			},
			"ALTER TABLE Ta SET ON DELETE NO ACTION",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "Ta",
				Alteration: SetOnDelete{Action: CascadeOnDelete},
				Position:   line(1),
			},
			"ALTER TABLE Ta SET ON DELETE CASCADE",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "Ta",
				Alteration: AlterColumn{
					Name: "Cg",
					Alteration: SetColumnType{
						Type: Type{Base: String, Len: MaxLen},
					},
				},
				Position: line(1),
			},
			"ALTER TABLE Ta ALTER COLUMN Cg STRING(MAX)",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "Ta",
				Alteration: AlterColumn{
					Name: "Ch",
					Alteration: SetColumnType{
						Type:    Type{Base: String, Len: MaxLen},
						NotNull: true,
						Default: StringLiteral("1"),
					},
				},
				Position: line(1),
			},
			"ALTER TABLE Ta ALTER COLUMN Ch STRING(MAX) NOT NULL DEFAULT (\"1\")",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "Ta",
				Alteration: AlterColumn{
					Name: "Ci",
					Alteration: SetColumnOptions{
						Options: ColumnOptions{
							AllowCommitTimestamp: boolAddr(false),
						},
					},
				},
				Position: line(1),
			},
			"ALTER TABLE Ta ALTER COLUMN Ci SET OPTIONS (allow_commit_timestamp = null)",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "Ta",
				Alteration: AlterColumn{
					Name: "Cj",
					Alteration: SetDefault{
						Default: StringLiteral("1"),
					},
				},
				Position: line(1),
			},
			"ALTER TABLE Ta ALTER COLUMN Cj SET DEFAULT (\"1\")",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "Ta",
				Alteration: AlterColumn{
					Name:       "Ck",
					Alteration: DropDefault{},
				},
				Position: line(1),
			},
			"ALTER TABLE Ta ALTER COLUMN Ck DROP DEFAULT",
			reparseDDL,
		},
		{
			&AlterTable{
				Name:       "WithRowDeletionPolicy",
				Alteration: DropRowDeletionPolicy{},
				Position:   line(1),
			},
			"ALTER TABLE WithRowDeletionPolicy DROP ROW DELETION POLICY",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "WithRowDeletionPolicy",
				Alteration: AddRowDeletionPolicy{
					RowDeletionPolicy: RowDeletionPolicy{
						Column:  ID("DelTimestamp"),
						NumDays: 30,
					},
				},
				Position: line(1),
			},
			"ALTER TABLE WithRowDeletionPolicy ADD ROW DELETION POLICY ( OLDER_THAN ( DelTimestamp, INTERVAL 30 DAY ))",
			reparseDDL,
		},
		{
			&AlterTable{
				Name: "WithRowDeletionPolicy",
				Alteration: ReplaceRowDeletionPolicy{
					RowDeletionPolicy: RowDeletionPolicy{
						Column:  ID("DelTimestamp"),
						NumDays: 30,
					},
				},
				Position: line(1),
			},
			"ALTER TABLE WithRowDeletionPolicy REPLACE ROW DELETION POLICY ( OLDER_THAN ( DelTimestamp, INTERVAL 30 DAY ))",
			reparseDDL,
		},
		{
			&AlterDatabase{
				Name: "dbname",
				Alteration: SetDatabaseOptions{Options: DatabaseOptions{
					EnableKeyVisualizer: func(b bool) *bool { return &b }(true),
				}},
				Position: line(1),
			},
			"ALTER DATABASE dbname SET OPTIONS (enable_key_visualizer=true)",
			reparseDDL,
		},
		{
			&AlterDatabase{
				Name: "dbname",
				Alteration: SetDatabaseOptions{Options: DatabaseOptions{
					OptimizerVersion: func(i int) *int { return &i }(2),
				}},
				Position: line(1),
			},
			"ALTER DATABASE dbname SET OPTIONS (optimizer_version=2)",
			reparseDDL,
		},
		{
			&AlterDatabase{
				Name: "dbname",
				Alteration: SetDatabaseOptions{Options: DatabaseOptions{
					VersionRetentionPeriod: func(s string) *string { return &s }("7d"),
					OptimizerVersion:       func(i int) *int { return &i }(2),
					EnableKeyVisualizer:    func(b bool) *bool { return &b }(true),
					DefaultLeader:          func(s string) *string { return &s }("europe-west1"),
				}},
				Position: line(1),
			},
			"ALTER DATABASE dbname SET OPTIONS (optimizer_version=2, version_retention_period='7d', enable_key_visualizer=true, default_leader='europe-west1')",
			reparseDDL,
		},
		{
			&AlterDatabase{
				Name: "dbname",
				Alteration: SetDatabaseOptions{Options: DatabaseOptions{
					VersionRetentionPeriod: func(s string) *string { return &s }(""),
					OptimizerVersion:       func(i int) *int { return &i }(0),
					EnableKeyVisualizer:    func(b bool) *bool { return &b }(false),
					DefaultLeader:          func(s string) *string { return &s }(""),
				}},
				Position: line(1),
			},
			"ALTER DATABASE dbname SET OPTIONS (optimizer_version=null, version_retention_period=null, enable_key_visualizer=null, default_leader=null)",
			reparseDDL,
		},
		{
			&CreateChangeStream{
				Name:           "csname",
				WatchAllTables: true,
				Options: ChangeStreamOptions{
					ValueCaptureType: func(s string) *string { return &s }("NEW_VALUES"),
				},
				Position: line(1),
			},
			"CREATE CHANGE STREAM csname FOR ALL OPTIONS (value_capture_type='NEW_VALUES')",
			reparseDDL,
		},
		{
			&CreateChangeStream{
				Name:           "csname",
				WatchAllTables: true,
				Options: ChangeStreamOptions{
					RetentionPeriod:  func(s string) *string { return &s }("7d"),
					ValueCaptureType: func(s string) *string { return &s }("NEW_VALUES"),
				},
				Position: line(1),
			},
			"CREATE CHANGE STREAM csname FOR ALL OPTIONS (retention_period='7d', value_capture_type='NEW_VALUES')",
			reparseDDL,
		},
		{
			&AlterChangeStream{
				Name: "csname",
				Alteration: AlterChangeStreamOptions{
					Options: ChangeStreamOptions{
						ValueCaptureType: func(s string) *string { return &s }("NEW_VALUES"),
					},
				},
				Position: line(1),
			},
			"ALTER CHANGE STREAM csname SET OPTIONS (value_capture_type='NEW_VALUES')",
			reparseDDL,
		},
		{
			&Insert{
				Table:   "Singers",
				Columns: []ID{ID("SingerId"), ID("FirstName"), ID("LastName")},
				Input:   Values{{IntegerLiteral(1), StringLiteral("Marc"), StringLiteral("Richards")}},
			},
			`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (1, "Marc", "Richards")`,
			reparseDML,
		},
		{
			&Delete{
				Table: "Ta",
				Where: ComparisonOp{
					LHS: ID("C"),
					Op:  Gt,
					RHS: IntegerLiteral(2),
				},
			},
			"DELETE FROM Ta WHERE C > 2",
			reparseDML,
		},
		{
			&Update{
				Table: "Ta",
				Items: []UpdateItem{
					{Column: "Cb", Value: IntegerLiteral(4)},
					{Column: "Ce", Value: StringLiteral("wow")},
					{Column: "Cf", Value: ID("Cg")},
					{Column: "Cg", Value: Null},
					{Column: "Ch", Value: nil},
				},
				Where: ID("Ca"),
			},
			`UPDATE Ta SET Cb = 4, Ce = "wow", Cf = Cg, Cg = NULL, Ch = DEFAULT WHERE Ca`,
			reparseDML,
		},
		{
			Query{
				Select: Select{
					List: []Expr{ID("A"), ID("B")},
					From: []SelectFrom{SelectFromTable{Table: "Table"}},
					Where: LogicalOp{
						LHS: ComparisonOp{
							LHS: ID("C"),
							Op:  Lt,
							RHS: StringLiteral("whelp"),
						},
						Op: And,
						RHS: IsOp{
							LHS: ID("D"),
							Neg: true,
							RHS: Null,
						},
					},
					ListAliases: []ID{"", "banana"},
				},
				Order: []Order{{Expr: ID("OCol"), Desc: true}},
				Limit: IntegerLiteral(1000),
			},
			`SELECT A, B AS banana FROM Table WHERE C < "whelp" AND D IS NOT NULL ORDER BY OCol DESC LIMIT 1000`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{ID("A")},
					From: []SelectFrom{SelectFromTable{
						Table: "Table",
						Hints: map[string]string{"FORCE_INDEX": "Idx"},
					}},
					Where: ComparisonOp{
						LHS: ID("B"),
						Op:  Eq,
						RHS: Param("b"),
					},
				},
			},
			`SELECT A FROM Table@{FORCE_INDEX=Idx} WHERE B = @b`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{ID("A")},
					From: []SelectFrom{SelectFromTable{
						Table: "Table",
						Hints: map[string]string{"FORCE_INDEX": "Idx", "GROUPBY_SCAN_OPTIMIZATION": "TRUE"},
					}},
					Where: ComparisonOp{
						LHS: ID("B"),
						Op:  Eq,
						RHS: Param("b"),
					},
				},
			},
			`SELECT A FROM Table@{FORCE_INDEX=Idx,GROUPBY_SCAN_OPTIMIZATION=TRUE} WHERE B = @b`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{IntegerLiteral(7)},
				},
			},
			`SELECT 7`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{Func{
						Name: "CAST",
						Args: []Expr{TypedExpr{Expr: IntegerLiteral(7), Type: Type{Base: String}}},
					}},
				},
			},
			`SELECT CAST(7 AS STRING)`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{Func{
						Name: "SAFE_CAST",
						Args: []Expr{TypedExpr{Expr: IntegerLiteral(7), Type: Type{Base: Date}}},
					}},
				},
			},
			`SELECT SAFE_CAST(7 AS DATE)`,
			reparseQuery,
		},
		{
			ComparisonOp{LHS: ID("X"), Op: NotBetween, RHS: ID("Y"), RHS2: ID("Z")},
			`X NOT BETWEEN Y AND Z`,
			reparseExpr,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						ID("Desc"),
					},
				},
			},
			"SELECT `Desc`",
			reparseQuery,
		},
		{
			DateLiteral(civil.Date{Year: 2014, Month: time.September, Day: 27}),
			`DATE '2014-09-27'`,
			reparseExpr,
		},
		{
			TimestampLiteral(time.Date(2014, time.September, 27, 12, 34, 56, 123456e3, latz)),
			`TIMESTAMP '2014-09-27 12:34:56.123456-07:00'`,
			reparseExpr,
		},
		{
			JSONLiteral(`{"a": 1}`),
			`JSON '{"a": 1}'`,
			reparseExpr,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						ID("A"), ID("B"),
					},
					From: []SelectFrom{
						SelectFromJoin{
							Type: InnerJoin,
							LHS:  SelectFromTable{Table: "Table1"},
							RHS:  SelectFromTable{Table: "Table2"},
							On: ComparisonOp{
								LHS: PathExp{"Table1", "A"},
								Op:  Eq,
								RHS: PathExp{"Table2", "A"},
							},
						},
					},
				},
			},
			"SELECT A, B FROM Table1 INNER JOIN Table2 ON Table1.A = Table2.A",
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						ID("A"), ID("B"),
					},
					From: []SelectFrom{
						SelectFromJoin{
							Type: InnerJoin,
							LHS: SelectFromJoin{
								Type: InnerJoin,
								LHS:  SelectFromTable{Table: "Table1"},
								RHS:  SelectFromTable{Table: "Table2"},
								On: ComparisonOp{
									LHS: PathExp{"Table1", "A"},
									Op:  Eq,
									RHS: PathExp{"Table2", "A"},
								},
							},
							RHS:   SelectFromTable{Table: "Table3"},
							Using: []ID{"X"},
						},
					},
				},
			},
			"SELECT A, B FROM Table1 INNER JOIN Table2 ON Table1.A = Table2.A INNER JOIN Table3 USING (X)",
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						Case{
							Expr: ID("X"),
							WhenClauses: []WhenClause{
								{Cond: IntegerLiteral(1), Result: StringLiteral("X")},
								{Cond: IntegerLiteral(2), Result: StringLiteral("Y")},
							},
							ElseResult: Null,
						},
					},
				},
			},
			`SELECT CASE X WHEN 1 THEN "X" WHEN 2 THEN "Y" ELSE NULL END`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						Case{
							WhenClauses: []WhenClause{
								{Cond: True, Result: StringLiteral("X")},
								{Cond: False, Result: StringLiteral("Y")},
							},
						},
					},
				},
			},
			`SELECT CASE WHEN TRUE THEN "X" WHEN FALSE THEN "Y" END`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						If{
							Expr:       ComparisonOp{LHS: IntegerLiteral(1), Op: Lt, RHS: IntegerLiteral(2)},
							TrueResult: True,
							ElseResult: False,
						},
					},
				},
			},
			`SELECT IF(1 < 2, TRUE, FALSE)`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						IfNull{
							Expr:       IntegerLiteral(10),
							NullResult: IntegerLiteral(0),
						},
					},
				},
			},
			`SELECT IFNULL(10, 0)`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						NullIf{
							Expr:        IntegerLiteral(10),
							ExprToMatch: IntegerLiteral(0),
						},
					},
				},
			},
			`SELECT NULLIF(10, 0)`,
			reparseQuery,
		},
		{
			Query{
				Select: Select{
					List: []Expr{
						Coalesce{
							ExprList: []Expr{
								StringLiteral("A"),
								Null,
								StringLiteral("C"),
							},
						},
					},
				},
			},
			`SELECT COALESCE("A", NULL, "C")`,
			reparseQuery,
		},
	}
	for _, test := range tests {
		sql := test.data.SQL()
		if sql != test.sql {
			t.Errorf("%v.SQL() wrong.\n got %s\nwant %s", test.data, sql, test.sql)
			continue
		}

		// As a confidence check, confirm that parsing the SQL produces the original input.
		data, err := test.reparse(sql)
		if err != nil {
			t.Errorf("Reparsing %q: %v", sql, err)
			continue
		}
		if !reflect.DeepEqual(data, test.data) {
			t.Errorf("Reparsing %q wrong.\n got %v\nwant %v", sql, data, test.data)
		}
	}
}
