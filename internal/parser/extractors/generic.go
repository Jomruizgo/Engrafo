package extractors

// GenericExtractor is a fallback for languages without a dedicated extractor.
// It extracts file-level nodes only, without symbol-level detail.
type GenericExtractor struct{}
