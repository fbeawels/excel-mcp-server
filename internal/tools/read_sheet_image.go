package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	z "github.com/Oudwins/zog"
	"github.com/devlights/goxcel"
	"github.com/devlights/goxcel/constants"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	imcp "github.com/fbeawels/excel-mcp-server/internal/mcp"
)

type ReadSheetImageArguments struct {
	FileAbsolutePath  string   `zog:"fileAbsolutePath"`
	SheetName         string   `zog:"sheetName"`
	Range             string   `zog:"range"`
	KnownPagingRanges []string `zog:"knownPagingRanges"`
}

var readSheetImageArgumentsSchema = z.Struct(z.Schema{
	"fileAbsolutePath":  z.String().Required(),
	"sheetName":         z.String().Required(),
	"range":             z.String(),
	"knownPagingRanges": z.Slice(z.String()),
})

func AddReadSheetImageTool(server *server.MCPServer) {
	server.AddTool(mcp.NewTool("read_sheet_image",
		mcp.WithDescription("[Windows only] Read data as an image from the Excel sheet with pagination."),
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
	), handleReadSheetImage)
}

func handleReadSheetImage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := ReadSheetImageArguments{}
	issues := readSheetImageArgumentsSchema.Parse(request.Params.Arguments, &args)
	if len(issues) != 0 {
		return imcp.NewToolResultZogIssueMap(issues), nil
	}
	return readSheetImage(args.FileAbsolutePath, args.SheetName, args.Range, args.KnownPagingRanges)
}

func readSheetImage(fileAbsolutePath string, sheetName string, rangeStr string, knownPagingRanges []string) (*mcp.CallToolResult, error) {
	quit := goxcel.MustInitGoxcel()
	defer quit()

	excel, release := goxcel.MustNewGoxcel()
	defer release()

	// ワークブックを開く
	workbooks := excel.MustWorkbooks()
	workbook, releaseWorkbook, err := workbooks.Open(fileAbsolutePath)
	if err != nil {
		return imcp.NewToolResultInvalidArgumentError(fmt.Errorf("failed to open workbook: %w", err).Error()), nil
	}
	defer releaseWorkbook()

	// ワークシートを取得
	sheets := workbook.MustWorkSheets()
	sheet, _ := sheets.Walk(func(sheet *goxcel.Worksheet, i int) error {
		name, err := sheet.Name()
		if err != nil {
			return nil
		}
		if name == sheetName {
			return fmt.Errorf("found")
		}
		return nil
	})
	if sheet == nil {
		return nil, fmt.Errorf("sheet not found: %s", sheetName)
	}
	foundSheetName, err := sheet.Name()
	if err != nil {
		return nil, fmt.Errorf("failed to get sheet name: %w", err)
	}

	pagingStrategy, err := NewGoxcelPagingStrategy(5000, sheet)
	if err != nil {
		return nil, err
	}
	pagingService := NewPagingRangeService(pagingStrategy)

	allRanges := pagingService.GetPagingRanges()
	if len(allRanges) == 0 {
		return imcp.NewToolResultInvalidArgumentError("no range available to read"), nil
	}

	var xlRange *goxcel.XlRange
	if rangeStr == "" {
		// range が指定されていない場合は最初の Range を使用
		startCol, startRow, endCol, endRow, err := ParseRange(allRanges[0])
		if err != nil {
			return nil, err
		}
		xlRange, err = sheet.Range(startRow, startCol, endRow, endCol)
		if err != nil {
			return nil, err
		}
	} else {
		// range が指定されている場合は指定された範囲を使用
		startCol, startRow, endCol, endRow, err := ParseRange(rangeStr)
		if err != nil {
			return imcp.NewToolResultInvalidArgumentError(fmt.Errorf("failed to parse range: %w", err).Error()), nil
		}
		// 必要な部分だけを選択するようにする
		xlRange, err = sheet.Range(startRow, startCol, endRow, endCol)
		if err != nil {
			return nil, fmt.Errorf("failed to create range: %w", err)
		}
	}
	currentRange, err := FetchRangeAddress(xlRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}
	remainingRanges := pagingService.FilterRemainingPagingRanges(allRanges, append(knownPagingRanges, currentRange))

	// 画像の取得用バッファの準備
	buf := new(bytes.Buffer)
	bufWriter := bufio.NewWriter(buf)
	err = xlRange.CopyPictureToFile(bufWriter, constants.XlScreen, constants.XlBitmap)
	if err != nil {
		return nil, fmt.Errorf("failed to copy range to image: %w", err)
	}
	err = bufWriter.Flush()
	if err != nil {
		return nil, fmt.Errorf("failed to flush buffer: %w", err)
	}
	// base64 エンコード
	base64Image := base64.StdEncoding.EncodeToString(buf.Bytes())

	text := "# Metadata\n"
	text += fmt.Sprintf("- sheet name: %s\n", foundSheetName)
	text += fmt.Sprintf("- read range: %s\n", currentRange)
	text += "# Notice\n"
	if len(remainingRanges) > 0 {
		text += "This sheet has more some ranges.\n"
		text += "To read the next range, you should specify 'range' and 'knownPagingRanges' arguments as follows.\n"
		text += fmt.Sprintf("`{ \"range\": \"%s\", \"knownPagingRanges\": [%s] }`", remainingRanges[0], "\""+strings.Join(append(knownPagingRanges, currentRange), "\", \"")+"\"")
	} else {
		text += "All ranges have been read.\n"
	}

	// 結果を返却
	return mcp.NewToolResultImage(
		text,
		base64Image,
		"image/png",
	), nil
}
