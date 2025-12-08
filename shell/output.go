package shell

import (
	"encoding/json"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/model"
)

type NodeJSON struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Version        int      `json:"version"`
	ModifiedClient string   `json:"modifiedClient"`
	CurrentPage    int      `json:"currentPage"`
	Starred        bool     `json:"starred"`
	Parent         string   `json:"parent"`
	Tags           []string `json:"tags"`
}

func NodeToJSON(node *model.Node) NodeJSON {
	return NodeJSON{
		ID:             node.Document.ID,
		Name:           node.Document.Name,
		Type:           node.Document.Type,
		Version:        node.Document.Version,
		ModifiedClient: node.Document.ModifiedClient,
		CurrentPage:    node.Document.CurrentPage,
		Starred:        node.Document.Starred,
		Parent:         node.Document.Parent,
		Tags:           node.Document.Tags,
	}
}

func displayNodesJSON(c *ishell.Context, nodes []*model.Node) error {
	jsonNodes := make([]NodeJSON, len(nodes))
	for i, node := range nodes {
		jsonNodes[i] = NodeToJSON(node)
	}

	output, err := json.MarshalIndent(jsonNodes, "", "  ")
	if err != nil {
		return err
	}

	c.Println(string(output))
	return nil
}
