package service

type tableCellAttrs struct {
	Colspan  int   `json:"colspan"`
	Rowspan  int   `json:"rowspan"`
	Colwidth []int `json:"colwidth"`
}

func readTableCell(n Node) (tableCellAttrs, error) {
	a := tableCellAttrs{Colspan: 1, Rowspan: 1}
	if readAttrs(n.Attrs, &a) != nil || a.Colspan < 1 || a.Colspan > 20 || a.Rowspan < 1 || a.Rowspan > 100 || len(a.Colwidth) > 0 && len(a.Colwidth) != a.Colspan {
		return a, ErrInvalid
	}
	for _, width := range a.Colwidth {
		if width < 16 || width > 2000 {
			return a, ErrInvalid
		}
	}
	return a, nil
}
func validateTable(n Node) error {
	if !noAttrs(n.Attrs) || len(n.Content) < 1 || len(n.Content) > 100 {
		return ErrInvalid
	}
	occupied := [20]int{}
	width := -1
	for index, row := range n.Content {
		if row.Type != "tableRow" || len(row.Content) < 1 || len(row.Content) > 20 {
			return ErrInvalid
		}
		column := 0
		for _, cell := range row.Content {
			if cell.Type != "tableCell" && cell.Type != "tableHeader" {
				return ErrInvalid
			}
			a, err := readTableCell(cell)
			if err != nil || a.Rowspan > len(n.Content)-index {
				return ErrInvalid
			}
			for column < 20 && occupied[column] > 0 {
				column++
			}
			if column+a.Colspan > 20 {
				return ErrInvalid
			}
			for i := column; i < column+a.Colspan; i++ {
				if occupied[i] != 0 {
					return ErrInvalid
				}
				occupied[i] = a.Rowspan
			}
			column += a.Colspan
		}
		rowWidth := 20
		for rowWidth > 0 && occupied[rowWidth-1] == 0 {
			rowWidth--
		}
		if width == -1 {
			width = rowWidth
		}
		if width != rowWidth {
			return ErrInvalid
		}
		for i := 0; i < width; i++ {
			if occupied[i] == 0 {
				return ErrInvalid
			}
			occupied[i]--
		}
	}
	return nil
}
