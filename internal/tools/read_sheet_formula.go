package tools

import (
	"context"
	"fmt"
	"strings"

	z "github.com/Oudwins/zog"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	imcp "github.com/fbeawels/excel-mcp-server/internal/mcp"
	"github.com/xuri/excelize/v2"
)

type ReadSheetFormulaArguments struct {
	FileAbsolutePath  string   `zog:"fileAbsolutePath"`
	SheetName         string   `zog:"sheetName"`
	Range             string   `zog:"range"`
	KnownPagingRanges []string `zog:"knownPagingRanges"`
}

var readSheetFormulaArgumentsSchema = z.Struct(z.Schema{
	"fileAbsolutePath":  z.String().Required(),
	"sheetName":         z.String().Required(),
	"range":             z.String(),
	"knownPagingRanges": z.Slice(z.String()),
})

func AddReadSheetFormulaTool(server *server.MCPServer) {
	server.AddTool(mcp.NewTool("read_sheet_formula",
		mcp.WithDescription("Read formulas from Excel sheet with pagination."),
		mcp.WithString("fileAbsolutePath",
			mcp.Required(),
			mcp.Description("Absolute path to the Excel file"),
		),
		mcp.WithString("sheetName",
			mcp.Required(),
			mcp.Description("Sheet name in the Excel file"),
		),
		mcp.WithString("range",
			mcp.Description("Range of cells to read in the Excel sheet (e.g., \"A1:C10\"). [default: first paging range]"),
		),
		imcp.WithArray("knownPagingRanges",
			mcp.Description("List of already read paging ranges"),
		),
	), handleReadSheetFormulaPaging)
}

func handleReadSheetFormulaPaging(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := ReadSheetFormulaArguments{}
	if issues := readSheetFormulaArgumentsSchema.Parse(request.Params.Arguments, &args); len(issues) != 0 {
		return imcp.NewToolResultZogIssueMap(issues), nil
	}
	return readSheetFormula(args.FileAbsolutePath, args.SheetName, args.Range, args.KnownPagingRanges)
}

func readSheetFormula(fileAbsolutePath string, sheetName string, valueRange string, knownPagingRanges []string) (*mcp.CallToolResult, error) {
	config, issues := LoadConfig()
	if issues != nil {
		return imcp.NewToolResultZogIssueMap(issues), nil
	}

	// ワークブックを開く
	workbook, err := excelize.OpenFile(fileAbsolutePath)
	if err != nil {
		return nil, err
	}
	defer workbook.Close()

	// シート名の確認
	if sheetName == "" {
		sheetName = workbook.GetSheetList()[0]
	}
	index, _ := workbook.GetSheetIndex(sheetName)
	if index == -1 {
		return imcp.NewToolResultInvalidArgumentError(fmt.Sprintf("sheet %s not found", sheetName)), nil
	}

	// ページング戦略の初期化
	strategy, err := NewFixedSizePagingStrategy(config.EXCEL_MCP_PAGING_CELLS_LIMIT, workbook, sheetName)
	if err != nil {
		return nil, err
	}
	pagingService := NewPagingRangeService(strategy)

	// 利用可能な範囲を取得
	allRanges := pagingService.GetPagingRanges()
	if len(allRanges) == 0 {
		return imcp.NewToolResultInvalidArgumentError("no range available to read"), nil
	}

	// 現在の範囲を決定
	currentRange := valueRange
	if currentRange == "" && len(allRanges) > 0 {
		currentRange = allRanges[0]
	}

	// 残りの範囲を計算
	remainingRanges := pagingService.FilterRemainingPagingRanges(allRanges, append(knownPagingRanges, currentRange))

	// 範囲の検証
	if err := pagingService.ValidatePagingRange(currentRange); err != nil {
		return imcp.NewToolResultInvalidArgumentError(fmt.Sprintf("invalid range: %v", err)), nil
	}

	// 範囲を解析
	startCol, startRow, endCol, endRow, err := ParseRange(currentRange)
	if err != nil {
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
	html += fmt.Sprintf("<li>read range: %s</li>\n", currentRange)
	html += "</ul>\n"
	html += "<h2>Notice</h2>\n"
	if len(remainingRanges) > 0 {
		html += "<p>This sheet has more some ranges.</p>\n"
		html += "<p>To read the next range, you should specify 'range' and 'knownPagingRanges' arguments as follows.</p>\n"
		html += fmt.Sprintf("<code>{ \"range\": \"%s\", \"knownPagingRanges\": [%s] }</code>\n", remainingRanges[0], "\""+strings.Join(append(knownPagingRanges, currentRange), "\", \"")+"\"")
	} else {
		html += "<p>All ranges have been read.</p>\n"
	}
	return mcp.NewToolResultText(html), nil
}
