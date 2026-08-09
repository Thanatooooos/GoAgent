package domain

type WikiGraphNode struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	InLinks  int    `json:"inLinks"`
	OutLinks int    `json:"outLinks"`
}

type WikiGraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Anchor string `json:"anchor"`
}

type WikiGraph struct {
	Nodes []WikiGraphNode `json:"nodes"`
	Edges []WikiGraphEdge `json:"edges"`
}
