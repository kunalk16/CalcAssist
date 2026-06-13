package tools

// Default returns a registry populated with all built-in tools.
func Default() *Registry {
	r := NewRegistry()
	r.Register(NewCreateFileTool())
	r.Register(NewListDirectoryTool())
	r.Register(NewReadFileTool())
	r.Register(NewSearchFilesTool())
	r.Register(NewCalculateTool())
	r.Register(NewStatisticsTool())
	r.Register(NewExcelToJSONTool())
	r.Register(NewReadPDFTool())
	r.Register(NewReadDocxTool())
	return r
}
