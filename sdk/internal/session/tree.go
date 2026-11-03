package session

import (
	"sort"
	"time"
)

// TreeNode is one entry in the session tree, with its resolved label and
// children oldest first.
type TreeNode struct {
	Entry          Entry       `json:"entry"`
	Children       []*TreeNode `json:"children"`
	Label          string      `json:"label,omitempty"`
	LabelTimestamp string      `json:"labelTimestamp,omitempty"`
}

// Tree returns the session's tree, as Pi's getTree builds it: an entry with no
// parent, a missing parent, or itself as parent is a root; children sort oldest
// first by timestamp, keeping the original order of unparsable timestamps; a
// label resolves from its latest label entry.
func (m *Manager) Tree() []*TreeNode {
	m.mu.Lock()
	entries := append([]Entry(nil), m.entries...)
	m.mu.Unlock()
	last := map[string]int{}
	for i, e := range entries {
		if e.ID != "" {
			last[e.ID] = i
		}
	}
	nodes := map[string]*TreeNode{}
	for i, e := range entries {
		if e.ID == "" || last[e.ID] != i {
			continue
		}
		nodes[e.ID] = &TreeNode{Entry: e}
	}
	var roots []*TreeNode
	for i, e := range entries {
		if e.ID == "" || last[e.ID] != i {
			continue
		}
		node := nodes[e.ID]
		parent := parentID(e)
		switch {
		case parent == "" || parent == e.ID:
			roots = append(roots, node)
		case nodes[parent] == nil:
			roots = append(roots, node)
		default:
			nodes[parent].Children = append(nodes[parent].Children, node)
		}
	}
	for _, node := range nodes {
		sort.SliceStable(node.Children, func(i, j int) bool {
			left, leftOK := treeTime(node.Children[i].Entry.Timestamp)
			right, rightOK := treeTime(node.Children[j].Entry.Timestamp)
			if !leftOK || !rightOK {
				return false
			}
			return left.Before(right)
		})
	}
	for _, e := range entries {
		if e.Type != "label" || e.TargetID == "" {
			continue
		}
		node := nodes[e.TargetID]
		if node == nil {
			continue
		}
		if e.Label == nil {
			node.Label = ""
			node.LabelTimestamp = ""
		} else {
			node.Label = *e.Label
			node.LabelTimestamp = e.Timestamp
		}
	}
	return roots
}

func treeTime(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
