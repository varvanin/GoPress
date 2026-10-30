package httpui

type editorTool struct{ Command, Label, Icon string }

var editorToolGroups = [][]editorTool{
	{{"undo", "Отменить", "undo-2"}, {"redo", "Повторить", "redo-2"}},
	{{"blockquote", "Цитата", "quote"}, {"codeBlock", "Блок кода", "square-code"}},
	{{"bold", "Жирный", "bold"}, {"italic", "Курсив", "italic"}, {"strike", "Зачёркнутый", "strikethrough"}, {"code", "Код", "code-xml"}, {"underline", "Подчёркнутый", "underline"}, {"highlight", "Выделить маркером", "highlighter"}, {"link", "Ссылка", "link"}, {"anchor", "Якорь", "hash"}},
	{{"superscript", "Верхний индекс", "superscript"}, {"subscript", "Нижний индекс", "subscript"}},
	{{"alignLeft", "По левому краю", "align-left"}, {"alignCenter", "По центру", "align-center"}, {"alignRight", "По правому краю", "align-right"}, {"alignJustify", "По ширине", "align-justify"}},
}

func activeTab(active bool) string {
	if active {
		return "page"
	}
	return "false"
}
