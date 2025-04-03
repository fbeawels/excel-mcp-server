package tools

import (
	"context"
	"fmt"

	z "github.com/Oudwins/zog"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	imcp "github.com/fbeawels/excel-mcp-server/internal/mcp"
	"github.com/xuri/excelize/v2"
)

type WriteSheetFormulaArguments struct {
	FileAbsolutePath string     `zog:"fileAbsolutePath"`
	SheetName        string     `zog:"sheetName"`
	Range            string     `zog:"range"`
	Formulas         [][]string `zog:"formulas"`
}

var writeSheetFormulaArgumentsSchema = z.Struct(z.Schema{
	"fileAbsolutePath": z.String().Required(),
	"sheetName":        z.String().Required(),
	"range":            z.String().Required(),
	"formulas":         z.Slice(z.Slice(z.String().HasPrefix("="))).Required(),
})

func AddWriteSheetFormulaTool(server *server.MCPServer) {
	server.AddTool(mcp.NewTool("write_sheet_formula",
		mcp.WithDescription("Write formulas to the Excel sheet"),
		mcp.WithString("fileAbsolutePath",
			mcp.Required(),
			mcp.Description("Absolute path to the Excel file"),
		),
		mcp.WithString("sheetName",
			mcp.Required(),
			mcp.Description("Sheet name in the Excel file"),
		),
		mcp.WithString("range",
			mcp.Required(),
			mcp.Description("Range of cells in the Excel sheet (e.g., \"A1:C10\")"),
		),
		imcp.WithArray("formulas",
			mcp.Required(),
			mcp.Description("Formulas to write to the Excel sheet (e.g., \"=A1+B1\")"),
		),
	), handleWriteSheetFormula)
}

func handleWriteSheetFormula(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := WriteSheetFormulaArguments{}
	issues := writeSheetFormulaArgumentsSchema.Parse(request.Params.Arguments, &args)
	if len(issues) != 0 {
		return imcp.NewToolResultZogIssueMap(issues), nil
	}
	return writeSheetFormula(args.FileAbsolutePath, args.SheetName, args.Range, args.Formulas)
}

func writeSheetFormula(fileAbsolutePath string, sheetName string, rangeStr string, formulas [][]string) (*mcp.CallToolResult, error) {
	workbook, err := excelize.OpenFile(fileAbsolutePath)
	if err != nil {
		return nil, err
	}
	defer workbook.Close()

	// シートの取得
	index, _ := workbook.GetSheetIndex(sheetName)
	if index == -1 {
		return imcp.NewToolResultInvalidArgumentError(fmt.Sprintf("sheet %s not found", sheetName)), nil
	}

	startCol, startRow, endCol, endRow, err := ParseRange(rangeStr)
	if err != nil {
		return imcp.NewToolResultInvalidArgumentError(err.Error()), nil
	}

	// データの整合性チェック
	rangeRowSize := endRow - startRow + 1
	if len(formulas) != rangeRowSize {
		return imcp.NewToolResultInvalidArgumentError(fmt.Sprintf("number of rows in data (%d) does not match range size (%d)", len(formulas), rangeRowSize)), nil
	}

	// データの書き込み
	for i, row := range formulas {
		rangeColumnSize := endCol - startCol + 1
		if len(row) != rangeColumnSize {
			return imcp.NewToolResultInvalidArgumentError(fmt.Sprintf("number of columns in row %d (%d) does not match range size (%d)", i, len(row), rangeColumnSize)), nil
		}
		for j, formula := range row {
			cell, err := excelize.CoordinatesToCellName(startCol+j, startRow+i)
			if err != nil {
				return nil, err
			}
			err = workbook.SetCellFormula(sheetName, cell, formula)
			if err != nil {
				return nil, err
			}
		}
	}

	if err := workbook.Save(); err != nil {
		return nil, err
	}

	// HTMLテーブルの生成
	table, err := CreateHTMLTableOfFormula(workbook, sheetName, startCol, startRow, endCol, endRow)
	if err != nil {
		return nil, err
	}
	html := "<h2>Sheet Formulas</h2>\n"
	html += *table + "\n"
	html += "<h2>Metadata</h2>\n"
	html += "<ul>\n"
	html += fmt.Sprintf("<li>sheet name: %s</li>\n", sheetName)
	html += fmt.Sprintf("<li>read range: %s</li>\n", rangeStr)
	html += "</ul>\n"
	html += "<h2>Notice</h2>\n"
	html += "<p>Formulas wrote successfully.</p>\n"

	return mcp.NewToolResultText(html), nil
}
