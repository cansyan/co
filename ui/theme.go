package ui

var Theme ColorTheme

type ColorTheme struct {
	Foreground string
	Background string
	Cursor     string
	Border     string
	Hover      string
	Selection  string
	Syntax     SyntaxStyle
}

type SyntaxStyle struct {
	Keyword      Style
	String       Style
	Comment      Style
	Number       Style
	Operator     Style
	FunctionName Style
	FunctionCall Style
}

var Breakers = ColorTheme{
	Foreground: "#333333", // grey3
	Background: "#fbffff", // white5 (extremely light cyan-white)
	Cursor:     "#fac863", // orange
	Border:     "#d9e0e4", // white2 (selection_border)
	Hover:      "#dae0e2", // white3
	Selection:  "#dae0e2", // white3 (line_highlight / selection)
	Syntax: SyntaxStyle{
		Keyword: Style{
			FG:         "#c594c5", // pink
			FontItalic: true,      // storage.type italic
		},
		String: Style{
			FG: "#89bd82", // green
		},
		Comment: Style{
			FG: "#999999", // grey2
		},
		Number: Style{
			FG: "#fac863", // orange
		},
		FunctionName: Style{
			FG: "#5fb3b3", // blue2 (entity.name.function)
		},
		FunctionCall: Style{
			FG: "#6699cc", // blue (variable.function)
		},
		Operator: Style{FG: "#F97B58"}, // red2
	},
}

var Mariana = ColorTheme{
	Foreground: "#d8dee9", // white3
	Background: "#303841", // blue3
	Cursor:     "#fac863", // orange
	Border:     "#65737e", // blue4 (selection_border)
	Hover:      "#4e5a65",
	Selection:  "#4e5a65", // blue2 (alpha handled by terminal blending)
	Syntax: SyntaxStyle{
		Keyword: Style{
			FG:         "#c594c5", // pink
			FontItalic: true,
		},
		String: Style{
			FG: "#99c794", // green
		},
		Comment: Style{
			FG: "#a7adba", // blue6
		},
		Number: Style{
			FG: "#fac863", // orange
		},
		FunctionName: Style{
			FG: "#5fb3b3", // blue5 (entity.name.function)
		},
		FunctionCall: Style{
			FG: "#6699cc", // blue (variable.function)
		},
		Operator: Style{FG: "#F97B58"}, // red2
	},
}
