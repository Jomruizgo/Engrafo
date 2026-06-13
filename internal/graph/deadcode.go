package graph

// OrphanNode is a node that never had any incoming edges.
type OrphanNode struct {
	Symbol    string
	Kind      string
	FilePath  string
	Language  string
}

// AbandonedNode is a node that once had incoming edges but now has none.
type AbandonedNode struct {
	Symbol              string
	Kind                string
	FilePath            string
	Language            string
	PeakIncomingEdges   int
	DaysSinceAbandoned  float64
}

// DeadcodeResult holds the output of a dead-code scan.
type DeadcodeResult struct {
	Orphans   []OrphanNode
	Abandoned []AbandonedNode
}

// Deadcode scans the graph for orphan and abandoned nodes.
// thresholdDays: only include nodes active for more than N days (0 = no filter).
// BLOQUEANTE: stub — implementación pendiente en feature/cg-deadcode green.
func (q *Querier) Deadcode(_ int) (*DeadcodeResult, error) {
	return nil, nil
}
