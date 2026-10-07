package imageevidence

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	imageOCRSummaryInstruction = "Summarize this document using only the document body and image OCR original text below. " +
		"Do not infer facts from image captions or invent missing details.\n\n" +
		"[Document body]\n"

	imageOCRSummaryTextHeader = "\n[Image OCR original text]\n"

	imageOCRTextTemplate = "[Image %d OCR original text]\n%s\n"
)
