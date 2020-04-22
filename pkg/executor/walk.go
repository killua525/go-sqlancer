package executor

import (
	"fmt"
	"strings"

	"github.com/chaos-mesh/private-wreck-it/pkg/types"
	"github.com/juju/errors"
	"github.com/pingcap/parser/ast"
	"github.com/pingcap/parser/model"
	"github.com/pingcap/parser/mysql"
	parserTypes "github.com/pingcap/parser/types"
)

func (e *Executor) walkDDLCreateTable(node *ast.CreateTableStmt, colTypes []string) (string, string, error) {
	table := fmt.Sprintf("%s_%s", "table", strings.Join(colTypes, "_"))

	idFieldType := parserTypes.NewFieldType(Type2Tp("int"))
	idFieldType.Flen = dataType2Len("int")
	idCol := &ast.ColumnDef{
		Name:    &ast.ColumnName{Name: model.NewCIStr(fmt.Sprintf("id"))},
		Tp:      idFieldType,
		Options: []*ast.ColumnOption{{Tp: ast.ColumnOptionAutoIncrement}},
	}
	node.Cols = append(node.Cols, idCol)
	makeConstraintPrimaryKey(node, "id")

	node.Table.Name = model.NewCIStr(table)
	for _, colType := range colTypes {
		fieldType := parserTypes.NewFieldType(Type2Tp(colType))
		fieldType.Flen = dataType2Len(colType)
		node.Cols = append(node.Cols, &ast.ColumnDef{
			Name: &ast.ColumnName{Name: model.NewCIStr(fmt.Sprintf("col_%s", colType))},
			Tp:   fieldType,
		})
	}
	sql, err := BufferOut(node)
	if err != nil {
		return "", "", err
	}
	return sql, table, errors.Trace(err)
}

func (e *Executor) walkDDLCreateIndex(node *ast.CreateIndexStmt) (string, error) {
	table := e.randTable()
	if table == nil {
		return "", errors.New("no table available")
	}
	node.Table.Name = model.NewCIStr(table.Table)
	node.IndexName = RdStringChar(5)
	for _, column := range table.Columns {
		name := column.Column
		if column.DataType == "text" {
			length := Rd(31) + 1
			name = fmt.Sprintf("%s(%d)", name, length)
		} else if column.DataType == "varchar" {
			length := 1
			if column.DataLen > 1 {
				maxLen := MinInt(column.DataLen, 32)
				length = Rd(maxLen-1) + 1
			}
			name = fmt.Sprintf("%s(%d)", name, length)
		}
		node.IndexPartSpecifications = append(node.IndexPartSpecifications,
			&ast.IndexPartSpecification{
				Column: &ast.ColumnName{
					Name: model.NewCIStr(name),
				},
			})
	}
	if len(node.IndexPartSpecifications) > 10 {
		node.IndexPartSpecifications = node.IndexPartSpecifications[:Rd(10)+1]
	}
	return BufferOut(node)
}

func (e *Executor) walkInsertStmtForTable(node *ast.InsertStmt, tableName string) (string, error) {
	table, ok := e.tables[tableName]
	if !ok {
		return "", errors.Errorf("table %s not exist", tableName)
	}
	node.Table.TableRefs.Left.(*ast.TableName).Name = model.NewCIStr(table.Table)
	columns := e.walkColumns(&node.Columns, table)
	e.walkLists(&node.Lists, columns)
	return BufferOut(node)
}

func (e *Executor) walkColumns(columns *[]*ast.ColumnName, table *types.Table) []*types.Column {
	var cols []*types.Column
	for _, column := range table.Columns {
		if column.Column == "id" {
			continue
		}
		*columns = append(*columns, &ast.ColumnName{
			Table: model.NewCIStr(column.Table),
			Name:  model.NewCIStr(column.Column),
		})
		cols = append(cols, column)
	}
	return cols
}

func (e *Executor) walkLists(lists *[][]ast.ExprNode, columns []*types.Column) {
	var noIDColumns []*types.Column
	for _, column := range columns {
		if column.Column != "id" {
			noIDColumns = append(noIDColumns, column)
		}
	}
	count := RdRange(10, 20)
	for i := 0; i < count; i++ {
		*lists = append(*lists, randList(noIDColumns))
	}
	// *lists = append(*lists, randor0(columns)...)
}

func randor0(cols []*types.Column) [][]ast.ExprNode {
	var (
		res     [][]ast.ExprNode
		zeroVal = ast.NewValueExpr(GenerateZeroDataItem(cols[0]), "", "")
		randVal = ast.NewValueExpr(GenerateDataItem(cols[0]), "", "")
		nullVal = ast.NewValueExpr(nil, "", "")
	)

	if len(cols) == 1 {
		res = append(res, []ast.ExprNode{zeroVal})
		res = append(res, []ast.ExprNode{randVal})
		res = append(res, []ast.ExprNode{nullVal})
		return res
	}
	for _, sub := range randor0(cols[1:]) {
		res = append(res, append([]ast.ExprNode{zeroVal}, sub...))
		res = append(res, append([]ast.ExprNode{randVal}, sub...))
		res = append(res, append([]ast.ExprNode{nullVal}, sub...))
	}
	return res
}

func randList(columns []*types.Column) []ast.ExprNode {
	var list []ast.ExprNode
	for _, column := range columns {
		// GenerateEnumDataItem
		switch Rd(3) {
		case 0:
			if column.HasOption(ast.ColumnOptionNotNull) {
				list = append(list, ast.NewValueExpr(GenerateEnumDataItem(column), "", ""))
			} else {
				list = append(list, ast.NewValueExpr(nil, "", ""))
			}
		default:
			list = append(list, ast.NewValueExpr(GenerateEnumDataItem(column), "", ""))
		}
	}
	return list
}

func (e *Executor) randTable() *types.Table {
	var tables []*types.Table
	for _, t := range e.tables {
		tables = append(tables, t)
	}
	if len(tables) == 0 {
		return nil
	}
	return tables[Rd(len(tables))]
}

// Type2Tp conver type string to tp byte
// TODO: complete conversion map
func Type2Tp(t string) byte {
	switch t {
	case "int":
		return mysql.TypeLong
	case "varchar":
		return mysql.TypeVarchar
	case "timestamp":
		return mysql.TypeTimestamp
	case "datetime":
		return mysql.TypeDatetime
	case "text":
		return mysql.TypeBlob
	case "float":
		return mysql.TypeFloat
	}
	return mysql.TypeNull
}