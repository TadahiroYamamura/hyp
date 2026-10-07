package research

import (
	"fmt"
	"time"
)

// 追加・移動・削除で親子関係が変わると、関係するほかのノードも書き換わることがある。
// そのため各操作は、保存が必要になったほかのノードを changed として返す。

// NewNode は追加するノードと、それに伴って変更されたノードを返す。
func NewNode(t *Tree, parentID, title string, now time.Time) (n *Node, changed []*Node, err error) {
	if title == "" {
		return nil, nil, fmt.Errorf("title must not be empty")
	}
	var parent *Node
	if parentID != "" {
		var ok bool
		if parent, ok = t.Node(parentID); !ok {
			return nil, nil, fmt.Errorf("parent %s not found", parentID)
		}
	}
	var id string
	for {
		if id, err = NewID(); err != nil {
			return nil, nil, err
		}
		if _, exists := t.Node(id); !exists {
			break
		}
	}
	if parent != nil && onChildAdded(parent) {
		changed = append(changed, parent)
	}
	return &Node{
		ID:        id,
		Parent:    parentID,
		Title:     title,
		Status:    StatusUntested,
		CreatedAt: now,
	}, changed, nil
}

// onChildAdded は、末端が子を持ったら status を消す。判定済みでも消すのは、
// 判定済みのノードにサブ仮説を足すのはその判定を疑い直したことを意味し、
// 後で子を除いても元の判定に戻すのは妥当でないため。以前の判定は git の履歴に残る。
func onChildAdded(p *Node) bool {
	if p.Status == "" {
		return false
	}
	p.Status = ""
	return true
}

// onChildRemoved は、最後の子を失って末端に戻ったノードに未判定の status を与える。
func onChildRemoved(t *Tree, p *Node) bool {
	if len(t.Children(p.ID)) != 1 || p.Status != "" {
		return false
	}
	p.Status = StatusUntested
	return true
}

// Edit は nil のフィールドを変更しない。
type Edit struct {
	Title       *string
	Description *string
	Method      *string
	Status      *string
	Combine     *string
	Condition   *string
}

func ApplyEdit(t *Tree, n *Node, e Edit) error {
	if e.Title != nil {
		if *e.Title == "" {
			return fmt.Errorf("title must not be empty")
		}
		n.Title = *e.Title
	}
	if e.Description != nil {
		n.Description = *e.Description
	}
	if e.Method != nil {
		n.Method = *e.Method
	}
	if e.Status != nil {
		s, err := ParseStatus(*e.Status)
		if err != nil {
			return err
		}
		if t.HasChildren(n.ID) {
			return fmt.Errorf("status can only be set on leaf hypotheses: %s has children and its conclusion is derived from them", n.ID)
		}
		n.Status = s
	}
	if e.Combine != nil {
		c, err := ParseCombine(*e.Combine)
		if err != nil {
			return err
		}
		if c != "" && !t.HasChildren(n.ID) {
			return fmt.Errorf("combine can only be set on hypotheses with children: %s has none", n.ID)
		}
		n.Combine = c
	}
	if e.Condition != nil {
		if *e.Condition != "" {
			p, ok := t.Node(n.Parent)
			if !ok || p.Combine != CombineCases {
				return fmt.Errorf("condition can only be set when the parent's combine is %q", CombineCases)
			}
		}
		n.Condition = *e.Condition
	}
	return nil
}

// Move は newParentID が空ならルートへ移動する。移動したノード自身も changed に含めて返す。
func Move(t *Tree, n *Node, newParentID string) (changed []*Node, err error) {
	if n.Parent == newParentID {
		if newParentID == "" {
			return nil, fmt.Errorf("%s is already a root", n.ID)
		}
		return nil, fmt.Errorf("%s is already under %s", n.ID, newParentID)
	}
	var newParent *Node
	if newParentID != "" {
		var ok bool
		if newParent, ok = t.Node(newParentID); !ok {
			return nil, fmt.Errorf("parent %s not found", newParentID)
		}
		if t.IsDescendant(newParentID, n.ID) {
			return nil, fmt.Errorf("cannot move %s under itself or its descendant %s", n.ID, newParentID)
		}
	}
	if old, ok := t.Node(n.Parent); ok && onChildRemoved(t, old) {
		changed = append(changed, old)
	}
	if newParent != nil && onChildAdded(newParent) {
		changed = append(changed, newParent)
	}
	n.Parent = newParentID
	return append(changed, n), nil
}

// Remove は削除に伴って変更されたノードを返す。ファイルの削除は呼び出し元が行う。
func Remove(t *Tree, n *Node) (changed []*Node, err error) {
	if t.HasChildren(n.ID) {
		return nil, fmt.Errorf("%s has children; remove or move them first", n.ID)
	}
	if p, ok := t.Node(n.Parent); ok && onChildRemoved(t, p) {
		changed = append(changed, p)
	}
	return changed, nil
}
