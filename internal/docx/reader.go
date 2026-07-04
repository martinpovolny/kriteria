// Package docx reads .docx (Office Open XML) files and extracts the document
// body as an ordered sequence of paragraphs and tables.
package docx

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Block is a paragraph or a table, in document order.
type Block struct {
	Paragraph *Paragraph
	Table     *Table
}

func (b Block) IsParagraph() bool { return b.Paragraph != nil }
func (b Block) IsTable() bool     { return b.Table != nil }

// Paragraph holds the style name (e.g. "Heading1") and concatenated text.
type Paragraph struct {
	Style string
	Text  string
}

// Table is a 2D grid of cells.
type Table struct {
	Rows []Row
}

type Row struct {
	Cells []Cell
}

// Cell holds the text of one table cell (paragraphs joined by \n).
type Cell struct {
	Text string
}

// Read opens a .docx file and returns the body content as blocks.
func Read(path string) ([]Block, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	defer r.Close()

	var f *zip.File
	for _, candidate := range r.File {
		if candidate.Name == "word/document.xml" {
			f = candidate
			break
		}
	}
	if f == nil {
		return nil, fmt.Errorf("word/document.xml not found")
	}

	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open document.xml: %w", err)
	}
	defer rc.Close()

	return parseBody(rc)
}

func parseBody(r io.Reader) ([]Block, error) {
	dec := xml.NewDecoder(r)

	// Skip to <w:body>
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "body" {
			break
		}
	}

	var blocks []Block
	for {
		tok, err := dec.Token()
		if err != nil {
			return blocks, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				p, err := parseParagraph(dec)
				if err != nil {
					return blocks, err
				}
				blocks = append(blocks, Block{Paragraph: &p})
			case "tbl":
				tbl, err := parseTable(dec)
				if err != nil {
					return blocks, err
				}
				blocks = append(blocks, Block{Table: &tbl})
			}
		case xml.EndElement:
			if t.Name.Local == "body" {
				return blocks, nil
			}
		}
	}
}

func parseParagraph(dec *xml.Decoder) (Paragraph, error) {
	var p Paragraph
	for {
		tok, err := dec.Token()
		if err != nil {
			return p, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pStyle":
				for _, attr := range t.Attr {
					if attr.Name.Local == "val" {
						p.Style = attr.Value
					}
				}
			case "t":
				text, err := readTextContent(dec)
				if err != nil {
					return p, err
				}
				p.Text += text
			case "tab":
				p.Text += "\t"
			case "br":
				p.Text += "\n"
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				return p, nil
			}
		}
	}
}

func readTextContent(dec *xml.Decoder) (string, error) {
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return sb.String(), err
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			return sb.String(), nil
		}
	}
}

func parseTable(dec *xml.Decoder) (Table, error) {
	var tbl Table
	for {
		tok, err := dec.Token()
		if err != nil {
			return tbl, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "tr" {
				row, err := parseRow(dec)
				if err != nil {
					return tbl, err
				}
				tbl.Rows = append(tbl.Rows, row)
			}
		case xml.EndElement:
			if t.Name.Local == "tbl" {
				return tbl, nil
			}
		}
	}
}

func parseRow(dec *xml.Decoder) (Row, error) {
	var row Row
	for {
		tok, err := dec.Token()
		if err != nil {
			return row, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "tc" {
				cell, err := parseCell(dec)
				if err != nil {
					return row, err
				}
				row.Cells = append(row.Cells, cell)
			}
		case xml.EndElement:
			if t.Name.Local == "tr" {
				return row, nil
			}
		}
	}
}

func parseCell(dec *xml.Decoder) (Cell, error) {
	var cell Cell
	var paraTexts []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return cell, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				p, err := parseParagraph(dec)
				if err != nil {
					return cell, err
				}
				paraTexts = append(paraTexts, p.Text)
			}
		case xml.EndElement:
			if t.Name.Local == "tc" {
				cell.Text = strings.Join(paraTexts, "\n")
				return cell, nil
			}
		}
	}
}
